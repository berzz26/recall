#!/usr/bin/env python3

"""L2.8 SmolVLM2-500M-Instruct segment scene understanding.

Reads one input JSON per video, loads SmolVLM2-500M-Video-Instruct once,
generates one description per segment from frame images, writes output JSON.
"""

import argparse
import json
import os
import shutil
import sys
import tempfile
import time


# Force unbuffered output when possible.
try:
    sys.stdout.reconfigure(line_buffering=True, write_through=True)
    sys.stderr.reconfigure(line_buffering=True, write_through=True)
except Exception:
    pass

MODEL_NAME = os.environ["VISION_MODEL"]
MODEL_PATH = os.environ["VISION_MODEL_PATH"]
MODEL_VERSION = os.environ["VISION_MODEL_VERSION"]


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
- Maximum 100 words.

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
            "Visible detector labels in this segment:\n"
            "none reported"
        )

    tracks = seg.get("tracks", []) or []

    if tracks:
        lines.append(
            "Tracks overlapping this segment "
            "(label, relative start, relative end):"
        )

        for t in tracks:
            lines.append(
                "- %s, %.1fs to %.1fs"
                % (
                    t.get("label", "?"),
                    float(t.get("start", 0.0)),
                    float(t.get("end", 0.0)),
                )
            )
    else:
        lines.append(
            "Tracks overlapping this segment: none"
        )

    events = seg.get("events", []) or []

    if events:
        lines.append(
            "Events in this segment "
            "(type, label, start, end):"
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
        lines.append(
            "Events in this segment: none"
        )

    append_block = (
        "\nStructured context (compact):\n"
        + "\n".join(lines)
    )

    return INSTRUCTION + append_block


def log_gpu_state(torch):
    try:
        cuda_available = torch.cuda.is_available()

        log(
            f"CUDA available: {cuda_available}"
        )

        if not cuda_available:
            return

        device_count = torch.cuda.device_count()

        log(
            f"CUDA device count: {device_count}"
        )

        for i in range(device_count):
            name = torch.cuda.get_device_name(i)
            props = torch.cuda.get_device_properties(i)

            log(
                f"GPU {i}: {name}"
            )

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
        log(
            f"Failed to inspect GPU state: {e}"
        )


def main():
    total_start = time.time()

    args = parse_args()

    tmp_root = tempfile.mkdtemp(
        prefix="recall-vision-"
    )

    log("========================================")
    log(
        "SmolVLM2-500M-Video-Instruct "
        "vision worker starting"
    )
    log("========================================")

    log(
        f"Python executable: {sys.executable}"
    )

    log(
        f"Python version: {sys.version.split()[0]}"
    )

    log(
        f"Working directory: {os.getcwd()}"
    )

    log(
        f"Input: {args.input}"
    )

    log(
        f"Output: {args.output}"
    )

    log(
        f"Model name: {MODEL_NAME}"
    )

    log(
        f"Model path: {MODEL_PATH}"
    )

    log(
        f"Model version: {MODEL_VERSION}"
    )

    # ------------------------------------------------------------
    # Check model directory
    # ------------------------------------------------------------

    log("Checking model directory...")

    if not os.path.exists(MODEL_PATH):
        log(
            f"ERROR: model directory does not exist: "
            f"{MODEL_PATH}"
        )

        shutil.rmtree(
            tmp_root,
            ignore_errors=True,
        )

        return 3

    if not os.path.isdir(MODEL_PATH):
        log(
            f"ERROR: model path is not a directory: "
            f"{MODEL_PATH}"
        )

        shutil.rmtree(
            tmp_root,
            ignore_errors=True,
        )

        return 3

    try:
        model_files = os.listdir(MODEL_PATH)

        log(
            f"Model directory exists: "
            f"{len(model_files)} entries"
        )

        for filename in model_files[:20]:
            log(
                f"Model file: {filename}"
            )

    except Exception as e:
        log(
            f"WARNING: failed to inspect model directory: {e}"
        )

    # ------------------------------------------------------------
    # Read input
    # ------------------------------------------------------------

    log("Reading input JSON...")

    try:
        with open(args.input) as f:
            data = json.load(f)

    except Exception as e:
        log(
            f"ERROR: failed to read input: {e}"
        )

        shutil.rmtree(
            tmp_root,
            ignore_errors=True,
        )

        return 2

    video_id = data.get("video_id", "")
    segments = data.get("segments", [])

    if not video_id or not isinstance(segments, list):
        log(
            "ERROR: video_id and segments are required"
        )

        shutil.rmtree(
            tmp_root,
            ignore_errors=True,
        )

        return 2

    log(
        f"Video ID: {video_id}"
    )

    log(
        f"Segments: {len(segments)}"
    )

    # ------------------------------------------------------------
    # PIL
    # ------------------------------------------------------------

    log("Importing PIL...")

    try:
        from PIL import Image

        log("PIL import OK")

    except Exception as e:
        log(
            f"ERROR: PIL import failed: {e}"
        )

        shutil.rmtree(
            tmp_root,
            ignore_errors=True,
        )

        return 3

    # ------------------------------------------------------------
    # Torch
    # ------------------------------------------------------------

    log("Importing torch...")

    try:
        import torch

        log(
            f"Torch import OK: {torch.__version__}"
        )

        log_gpu_state(torch)

    except Exception as e:
        log(
            f"ERROR: torch import failed: {e}"
        )

        shutil.rmtree(
            tmp_root,
            ignore_errors=True,
        )

        return 3

    # ------------------------------------------------------------
    # Transformers
    # ------------------------------------------------------------

    log("Importing transformers...")

    try:
        import transformers

        log(
            f"Transformers import OK: "
            f"{transformers.__version__}"
        )

        from transformers import (
            AutoProcessor,
            AutoModelForImageTextToText,
        )

        log(
            "AutoProcessor import OK"
        )

        log(
            "AutoModelForImageTextToText import OK"
        )

    except Exception as e:
        log(
            f"ERROR: transformers import failed: {e}"
        )

        shutil.rmtree(
            tmp_root,
            ignore_errors=True,
        )

        return 3

    # ------------------------------------------------------------
    # Model load
    # ------------------------------------------------------------

    log("========================================")
    log("STARTING MODEL LOAD")
    log("========================================")

    model_start = time.time()

    try:
        log(
            f"Loading processor from: {MODEL_PATH}"
        )

        processor_start = time.time()

        processor = AutoProcessor.from_pretrained(
            MODEL_PATH
        )

        log(
            f"Processor loaded in "
            f"{time.time() - processor_start:.2f}s"
        )

        log(
            f"Loading model from: {MODEL_PATH}"
        )

        log(
            "Using CPU float32 inference"
        )

        model = AutoModelForImageTextToText.from_pretrained(
            MODEL_PATH,
            torch_dtype=torch.float32,
        )

        model.eval()

        log(
            f"MODEL LOAD COMPLETE in "
            f"{time.time() - model_start:.2f}s"
        )

        log(
            f"Model device: "
            f"{getattr(model, 'device', 'unknown')}"
        )

        log_gpu_state(torch)

    except Exception as e:
        log(
            f"ERROR: failed to load model: {e}"
        )

        shutil.rmtree(
            tmp_root,
            ignore_errors=True,
        )

        return 3

    log("========================================")
    log("MODEL READY")
    log("========================================")

    descriptions = []

    # ------------------------------------------------------------
    # Segments
    # ------------------------------------------------------------

    try:
        for segment_index, seg in enumerate(
            segments,
            start=1,
        ):
            segment_start = time.time()

            segment_id = seg.get(
                "segment_id",
                "",
            )

            frames = seg.get(
                "frames",
                [],
            ) or []

            log("----------------------------------------")
            log(
                f"Segment "
                f"{segment_index}/{len(segments)}"
            )

            log(
                f"Segment ID: {segment_id}"
            )

            log(
                f"Frame count: {len(frames)}"
            )

            if not segment_id or not frames:
                log(
                    "ERROR: invalid segment: "
                    "id and frames required"
                )

                shutil.rmtree(
                    tmp_root,
                    ignore_errors=True,
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
                path = fr.get(
                    "path",
                    "",
                )

                if not path:
                    log(
                        f"ERROR: frame "
                        f"{frame_index} missing path"
                    )

                    shutil.rmtree(
                        tmp_root,
                        ignore_errors=True,
                    )

                    return 2

                log(
                    f"Loading frame "
                    f"{frame_index}/{len(frames)}: "
                    f"{path}"
                )

                frame_start = time.time()

                try:
                    img = Image.open(
                        path
                    ).convert("RGB")

                    log(
                        f"Frame loaded: "
                        f"{img.width}x{img.height} "
                        f"in "
                        f"{time.time() - frame_start:.3f}s"
                    )

                except Exception as e:
                    log(
                        f"ERROR: failed to open "
                        f"frame {path}: {e}"
                    )

                    shutil.rmtree(
                        tmp_root,
                        ignore_errors=True,
                    )

                    return 2

                images.append(img)

            log(
                f"All {len(images)} frames loaded"
            )

            # ----------------------------------------------------
            # Prompt
            # ----------------------------------------------------

            log("Building prompt...")

            prompt_start = time.time()

            prompt = build_context(seg)

            log(
                f"Prompt built in "
                f"{time.time() - prompt_start:.3f}s"
            )

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

            log(
                "Applying chat template..."
            )

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

            log(
                f"Formatted prompt length: "
                f"{len(text)} characters"
            )

            # ----------------------------------------------------
            # Processor
            # ----------------------------------------------------

            log(
                "STARTING IMAGE PROCESSING"
            )

            processor_start = time.time()

            inputs = processor(
                text=[text],
                images=images,
                padding=True,
                return_tensors="pt",
            )

            log(
                f"IMAGE PROCESSING COMPLETE in "
                f"{time.time() - processor_start:.2f}s"
            )

            tensor_shapes = {}

            for key, value in inputs.items():
                if hasattr(value, "shape"):
                    tensor_shapes[key] = tuple(
                        value.shape
                    )

            log(
                f"Input tensors: {tensor_shapes}"
            )

            log(
                "Moving inputs to model device..."
            )

            move_start = time.time()

            inputs = inputs.to(
                model.device
            )

            log(
                f"Inputs moved in "
                f"{time.time() - move_start:.3f}s"
            )

            # ----------------------------------------------------
            # Generation
            # ----------------------------------------------------

            log("========================================")
            log(
                f"STARTING GENERATION: "
                f"{segment_id}"
            )
            log(
                "CPU inference may take several minutes."
            )
            log("========================================")

            generation_start = time.time()

            with torch.inference_mode():
                generated = model.generate(
                    **inputs,
                    max_new_tokens=128,
                    do_sample=False,
                )

            generation_time = (
                time.time() - generation_start
            )

            log(
                f"GENERATION COMPLETE in "
                f"{generation_time:.2f}s"
            )

            log(
                f"Generated tensor shape: "
                f"{tuple(generated.shape)}"
            )

            # ----------------------------------------------------
            # Decode
            # ----------------------------------------------------

            log(
                "Decoding generated tokens..."
            )

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

                shutil.rmtree(
                    tmp_root,
                    ignore_errors=True,
                )

                return 3

            log(
                f"Description generated "
                f"({len(desc)} chars):"
            )

            log(
                desc
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
                f"in "
                f"{time.time() - segment_start:.2f}s"
            )

    except SystemExit:
        raise

    except Exception as e:
        log(
            f"ERROR: inference failed: {e}"
        )

        shutil.rmtree(
            tmp_root,
            ignore_errors=True,
        )

        return 3

    # ------------------------------------------------------------
    # Write output
    # ------------------------------------------------------------

    try:
        log(
            "Writing output JSON..."
        )

        out = {
            "video_id": video_id,
            "descriptions": descriptions,
        }

        with open(args.output, "w") as f:
            json.dump(
                out,
                f,
            )

        log(
            f"Output written: {args.output}"
        )

    except Exception as e:
        log(
            f"ERROR: failed to write output: {e}"
        )

        shutil.rmtree(
            tmp_root,
            ignore_errors=True,
        )

        return 2

    shutil.rmtree(
        tmp_root,
        ignore_errors=True,
    )

    log("========================================")
    log(
        f"VISION WORKER COMPLETE in "
        f"{time.time() - total_start:.2f}s"
    )
    log(
        f"Descriptions generated: "
        f"{len(descriptions)}"
    )
    log("========================================")

    return 0


if __name__ == "__main__":
    sys.exit(main())