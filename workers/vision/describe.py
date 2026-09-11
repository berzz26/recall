#!/usr/bin/env python3

"""L2.8 Qwen3-VL segment scene understanding.

Reads one input JSON per video, loads Qwen/Qwen3-VL-2B-Instruct once,
generates one description per segment from frame images, writes output JSON.
"""

import argparse
import json
import os
import sys
import time


MODEL_NAME = "/models/Qwen3-VL-2B-Instruct"
MODEL_VERSION = "2B-Instruct"


INSTRUCTION = """You are describing a CCTV video segment for a video search system.

The supplied images are chronological representative frames from one
continuous video segment.

Describe what is visibly happening across the segment.

The images are the primary evidence.

Structured detector, tracking, and event metadata is supplemental
context only. It may be incomplete or incorrect. Do not make a claim
merely because metadata says something exists. Verify visual claims
against the images.

Rules:
- Describe only visible evidence.
- Do not identify people.
- Do not infer names or identities.
- Do not infer intentions or motives.
- Do not invent actions that are not visually supported.
- Do not speculate about why something is happening.
- Use neutral language.
- Mention important objects, people, vehicles, and scene activity.
- Describe meaningful movement or changes across the supplied frames.
- If people are present, describe their visible activity without
  identifying them.
- Do not mention detector confidence scores.
- Do not mention track IDs.
- Do not mention internal metadata.
- Do not mention that you are an AI.
- Do not say that something is present if it is not visually supported.
- If the scene is static, describe the visible scene concisely.
- Do not produce a frame-by-frame list.
- Produce one concise paragraph.
- Maximum 120 words.

Return only the description."""


def log(message):
    timestamp = time.strftime("%Y-%m-%d %H:%M:%S")
    print(
        f"[{timestamp}] [VISION] {message}",
        file=sys.stderr,
        flush=True,
    )


def parse_args():
    p = argparse.ArgumentParser()
    p.add_argument("--input", required=True)
    p.add_argument("--output", required=True)
    return p.parse_args()


def build_context(seg):
    lines = []

    dets = seg.get("detections", []) or []

    if dets:
        labels = sorted(
            {
                str(d.get("label", ""))
                for d in dets
                if d.get("label")
            }
        )
        labels = [l for l in labels if l]

        if labels:
            lines.append(
                "Visible detector labels in this segment:\n"
                + ", ".join(labels)
            )
    else:
        lines.append(
            "Visible detector labels in this segment:\nnone reported"
        )

    tracks = seg.get("tracks", []) or []

    if tracks:
        lines.append(
            "Tracks overlapping this segment "
            "(label, track index, relative start, relative end):"
        )

        for t in tracks:
            lines.append(
                "- %s, track %s, %.1fs to %.1fs"
                % (
                    t.get("label", "?"),
                    t.get("track_index", "?"),
                    float(t.get("start", 0.0)),
                    float(t.get("end", 0.0)),
                )
            )
    else:
        lines.append("Tracks overlapping this segment: none")

    events = seg.get("events", []) or []

    if events:
        lines.append(
            "Events in this segment (type, label, start, end):"
        )

        for e in events:
            end = e.get("end")
            end_s = (
                "none"
                if end is None
                else "%.1fs" % float(end)
            )

            lines.append(
                "- %s, %s, %.1fs to %s"
                % (
                    e.get("event_type", "?"),
                    e.get("label", "?"),
                    float(e.get("start", 0.0)),
                    end_s,
                )
            )
    else:
        lines.append("Events in this segment: none")

    grounding = (
        "The images are the primary evidence.\n\n"
        "Structured detector, track, and event information is only\n"
        "supplemental context and may be incomplete or incorrect.\n\n"
        "Do not claim something merely because it appears in the\n"
        "structured metadata.\n\n"
        "Only describe things supported by the supplied images."
    )

    return (
        INSTRUCTION
        + "\n\n"
        + "\n".join(lines)
        + "\n\n"
        + grounding
    )


def log_gpu_state(torch):
    try:
        cuda_available = torch.cuda.is_available()

        log(f"CUDA available: {cuda_available}")

        if not cuda_available:
            return

        device_count = torch.cuda.device_count()
        log(f"CUDA device count: {device_count}")

        for i in range(device_count):
            name = torch.cuda.get_device_name(i)
            props = torch.cuda.get_device_properties(i)

            log(f"GPU {i}: {name}")
            log(
                f"GPU {i} total memory: "
                f"{props.total_memory / (1024 ** 3):.2f} GB"
            )

            allocated = torch.cuda.memory_allocated(i)
            reserved = torch.cuda.memory_reserved(i)

            log(
                f"GPU {i} memory: "
                f"{allocated / (1024 ** 3):.2f} GB allocated, "
                f"{reserved / (1024 ** 3):.2f} GB reserved"
            )

    except Exception as e:
        log(f"Failed to inspect GPU state: {e}")


def main():
    total_start = time.time()

    args = parse_args()

    log("========================================")
    log("Qwen3-VL vision worker starting")
    log("========================================")

    log(f"Python executable: {sys.executable}")
    log(f"Python version: {sys.version.split()[0]}")
    log(f"Working directory: {os.getcwd()}")
    log(f"Input: {args.input}")
    log(f"Output: {args.output}")
    log(f"Model path: {MODEL_NAME}")

    # ------------------------------------------------------------
    # Check model
    # ------------------------------------------------------------

    log("Checking model directory...")

    if not os.path.exists(MODEL_NAME):
        log(f"ERROR: model directory does not exist: {MODEL_NAME}")
        return 3

    if not os.path.isdir(MODEL_NAME):
        log(f"ERROR: model path is not a directory: {MODEL_NAME}")
        return 3

    try:
        files = os.listdir(MODEL_NAME)

        log(
            f"Model directory exists "
            f"({len(files)} entries)"
        )

        log(
            "Model files: "
            + ", ".join(files[:20])
        )

    except Exception as e:
        log(f"WARNING: could not inspect model directory: {e}")

    # ------------------------------------------------------------
    # Read input
    # ------------------------------------------------------------

    log("Reading input JSON...")

    try:
        with open(args.input) as f:
            data = json.load(f)

    except Exception as e:
        log(f"ERROR: failed to read input: {e}")
        return 2

    video_id = data.get("video_id", "")
    segments = data.get("segments", [])

    if not video_id or not isinstance(segments, list):
        log("ERROR: video_id and segments are required")
        return 2

    log(f"Video ID: {video_id}")
    log(f"Segments: {len(segments)}")

    # ------------------------------------------------------------
    # Imports
    # ------------------------------------------------------------

    log("Importing PIL...")

    try:
        from PIL import Image
        log("PIL import OK")
    except Exception as e:
        log(f"ERROR: PIL import failed: {e}")
        return 3

    log("Importing torch...")

    try:
        import torch

        log(f"Torch import OK: {torch.__version__}")
        log_gpu_state(torch)

    except Exception as e:
        log(f"ERROR: torch import failed: {e}")
        return 3

    log("Importing transformers...")

    try:
        import transformers

        log(
            f"Transformers import OK: "
            f"{transformers.__version__}"
        )

        from transformers import (
            Qwen3VLForConditionalGeneration,
            AutoProcessor,
        )

        log("Qwen3VLForConditionalGeneration import OK")
        log("AutoProcessor import OK")

    except Exception as e:
        log(f"ERROR: transformers import failed: {e}")
        return 3

    # ------------------------------------------------------------
    # Load model
    # ------------------------------------------------------------

    log("========================================")
    log("STARTING MODEL LOAD")
    log("========================================")

    model_start = time.time()

    try:
        log(
            "Calling "
            "Qwen3VLForConditionalGeneration.from_pretrained()..."
        )

        model = Qwen3VLForConditionalGeneration.from_pretrained(
            MODEL_NAME,
            dtype="auto",
            device_map="auto",
        )

        log(
            f"MODEL LOAD COMPLETE "
            f"in {time.time() - model_start:.2f}s"
        )

        log(
            f"Model device: "
            f"{getattr(model, 'device', 'unknown')}"
        )

        log(
            f"Model device map: "
            f"{getattr(model, 'hf_device_map', 'unknown')}"
        )

        log_gpu_state(torch)

    except Exception as e:
        log(f"ERROR: failed to load model: {e}")
        return 3

    # ------------------------------------------------------------
    # Load processor
    # ------------------------------------------------------------

    log("Loading processor...")

    processor_start = time.time()

    try:
        processor = AutoProcessor.from_pretrained(
            MODEL_NAME
        )

        log(
            f"Processor loaded in "
            f"{time.time() - processor_start:.2f}s"
        )

    except Exception as e:
        log(f"ERROR: failed to load processor: {e}")
        return 3

    log("========================================")
    log("MODEL READY")
    log("========================================")

    # ------------------------------------------------------------
    # Process segments
    # ------------------------------------------------------------

    descriptions = []

    try:
        for segment_index, seg in enumerate(
            segments,
            start=1,
        ):
            segment_start = time.time()

            segment_id = seg.get("segment_id", "")
            frames = seg.get("frames", []) or []

            log(
                f"Segment {segment_index}/{len(segments)} "
                f"starting: id={segment_id}, "
                f"frames={len(frames)}"
            )

            if not segment_id or not frames:
                log(
                    "ERROR: invalid segment: "
                    "id and frames required"
                )
                return 2

            # ----------------------------------------------------
            # Load frames
            # ----------------------------------------------------

            images = []

            for frame_index, fr in enumerate(
                frames,
                start=1,
            ):
                path = fr.get("path", "")

                if not path:
                    log(
                        f"ERROR: frame {frame_index} "
                        f"missing path"
                    )
                    return 2

                log(
                    f"Loading frame "
                    f"{frame_index}/{len(frames)}: {path}"
                )

                frame_start = time.time()

                try:
                    img = Image.open(path).convert("RGB")

                    log(
                        f"Frame loaded: "
                        f"{img.width}x{img.height} "
                        f"in {time.time() - frame_start:.3f}s"
                    )

                except Exception as e:
                    log(
                        f"ERROR: failed to open frame "
                        f"{path}: {e}"
                    )
                    return 2

                images.append(img)

            log(f"All {len(images)} frames loaded")

            # ----------------------------------------------------
            # Build prompt
            # ----------------------------------------------------

            log("Building prompt...")

            prompt = build_context(seg)

            log(
                f"Prompt length: "
                f"{len(prompt)} characters"
            )

            content = [
                {
                    "type": "image",
                    "image": img,
                }
                for img in images
            ]

            content.append(
                {
                    "type": "text",
                    "text": prompt,
                }
            )

            messages = [
                {
                    "role": "user",
                    "content": content,
                }
            ]

            # ----------------------------------------------------
            # Chat template
            # ----------------------------------------------------

            log("Applying chat template...")

            template_start = time.time()

            text = processor.apply_chat_template(
                messages,
                tokenize=False,
                add_generation_prompt=True,
            )

            log(
                f"Chat template complete in "
                f"{time.time() - template_start:.3f}s"
            )

            # ----------------------------------------------------
            # Processor
            # ----------------------------------------------------

            log("Running processor on frames...")

            processor_start = time.time()

            inputs = processor(
                text=[text],
                images=images,
                padding=True,
                return_tensors="pt",
            )

            log(
                f"Processor complete in "
                f"{time.time() - processor_start:.3f}s"
            )

            log(
                "Input tensors: "
                + str(
                    {
                        k: tuple(v.shape)
                        for k, v in inputs.items()
                        if hasattr(v, "shape")
                    }
                )
            )

            log("Moving inputs to model device...")

            inputs = inputs.to(model.device)

            log("Inputs moved to model device")

            log_gpu_state(torch)

            # ----------------------------------------------------
            # Generation
            # ----------------------------------------------------

            log("----------------------------------------")
            log(f"STARTING GENERATION: {segment_id}")
            log("----------------------------------------")

            generation_start = time.time()

            generated = model.generate(
                **inputs,
                max_new_tokens=160,
                do_sample=False,
            )

            log(
                f"GENERATION COMPLETE in "
                f"{time.time() - generation_start:.2f}s"
            )

            log(
                f"Generated tensor shape: "
                f"{tuple(generated.shape)}"
            )

            log_gpu_state(torch)

            # ----------------------------------------------------
            # Decode
            # ----------------------------------------------------

            log("Decoding generated tokens...")

            trimmed = generated[
                :,
                inputs.input_ids.shape[1]:
            ]

            desc = processor.batch_decode(
                trimmed,
                skip_special_tokens=True,
                clean_up_tokenization_spaces=False,
            )[0].strip()

            if not desc:
                log(
                    f"ERROR: empty description "
                    f"for segment {segment_id}"
                )
                return 3

            log(
                f"Description generated "
                f"({len(desc)} chars): {desc}"
            )

            descriptions.append(
                {
                    "segment_id": segment_id,
                    "description": desc,
                    "model_name": MODEL_NAME,
                    "model_version": MODEL_VERSION,
                }
            )

            log(
                f"Segment {segment_id} COMPLETE "
                f"in {time.time() - segment_start:.2f}s"
            )

    except SystemExit:
        raise

    except Exception as e:
        log(f"ERROR: inference failed: {e}")
        return 3

    # ------------------------------------------------------------
    # Write output
    # ------------------------------------------------------------

    log("Writing output JSON...")

    out = {
        "video_id": video_id,
        "descriptions": descriptions,
    }

    try:
        with open(args.output, "w") as f:
            json.dump(out, f)

        log(f"Output written: {args.output}")

    except Exception as e:
        log(f"ERROR: failed to write output: {e}")
        return 2

    # ------------------------------------------------------------
    # Done
    # ------------------------------------------------------------

    log("========================================")
    log(
        f"VISION WORKER COMPLETE "
        f"in {time.time() - total_start:.2f}s"
    )
    log(f"Descriptions generated: {len(descriptions)}")
    log("========================================")

    return 0


if __name__ == "__main__":
    sys.exit(main())