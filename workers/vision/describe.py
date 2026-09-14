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

MODEL_NAME = os.getenv("VISION_MODEL", "HuggingFaceTB/SmolVLM-500M-Instruct")
MODEL_PATH = os.getenv("VISION_MODEL_PATH", os.getenv("VISION_MODEL", "/models/SmolVLM2-500M-Video-Instruct"))
# Handle case where VISION_MODEL is model name not path
if MODEL_PATH == MODEL_NAME and "/" not in MODEL_PATH:
    MODEL_PATH = os.getenv("VISION_MODEL_PATH", "/models/SmolVLM2-500M-Video-Instruct")
MODEL_VERSION = os.getenv("VISION_MODEL_VERSION", "500M-Instruct")
try:
    MAX_OUTPUT_TOKENS = int(os.getenv("VISION_MAX_OUTPUT_TOKENS", "256"))
except Exception:
    MAX_OUTPUT_TOKENS = 256


INSTRUCTION = """Generate a concise semantic description optimized for video retrieval for this CCTV video segment.

The supplied images are chronological representative frames from one continuous video segment.
You also receive bounded structured context: current detections/tracks/events for this segment, plus bounded temporal context (recent prior segment summaries, persistent tracks active before this segment, and recent prior events). This temporal context is CONTEXT FOR YOU to understand continuity — do NOT repeat it verbatim, do NOT list tracks/events, and do NOT expose track IDs, detector labels, confidence scores, or internal metadata in your answer. Use it only to determine whether an entity is continuing from a previous segment.

The images are the primary evidence. Structured detector, tracking, and event metadata is supplemental context only. It may be incomplete or incorrect. Do not make a claim merely because metadata says something exists. Verify visual claims against the images.

Describe in natural language as one concise paragraph, approximately 50-100 words, no bullets, no JSON, no frame-by-frame list. Return only the description.

Content to cover when visibly supported and useful for retrieval:
- Scene/context: what kind of environment/scene is visible, important objects/vehicles/people.
- Salient entities: describe visually meaningful people/objects when relevant, including useful visible attributes (clothing, color, carried objects, approximate location) and where they are in the scene.
- Activities/actions: what important entities are visibly doing, including meaningful interactions or actions.
- Temporal change: meaningful movement, appearance, disappearance, or state changes across the supplied frames.
- Cross-segment continuity: when tracking indicates the same entity continues from a previous segment AND the frames are visually consistent, use natural continuity language such as "the same person continues..." or "the person in the blue jacket moves..." Do NOT mention track IDs.

Rules:
- Describe only visible evidence.
- Prioritize salient entities and actions useful for semantic retrieval. Do NOT force every detected person/track into the paragraph.
- Do not identify real people or infer identities, names, intentions, or motives.
- Do not invent actions that are not visually supported and do not speculate about why something is happening.
- Use neutral language.
- If the scene is static, describe the visible scene concisely.
- Do not say that something is present if it is not visually supported.
- Do not mention detector confidence scores, track IDs, detector labels unless naturally useful, or internal implementation details.
- Do not mention that you are an AI.
- One paragraph, approximately 50-100 words. Maximum 100 words.

Return only the description."""


def _log(level, msg, **fields):
    """Production-grade structured JSON logger to stderr."""
    # Auto-detect level from msg prefix if not explicitly set
    if msg.startswith("ERROR:"):
        level = "ERROR"
    elif msg.startswith("WARNING:"):
        level = "WARN"
    record = {
        "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "level": level,
        "component": "vision",
        "msg": msg,
    }
    if fields:
        # Ensure JSON serializable
        for k, v in fields.items():
            try:
                json.dumps(v)
                record[k] = v
            except Exception:
                record[k] = str(v)
    print(json.dumps(record), file=sys.stderr, flush=True)


def log(message, level="INFO", **fields):
    """Backwards-compatible wrapper: log(message) -> INFO JSON."""
    # Allow caller to pass level via message prefix
    if isinstance(message, str) and message.startswith("ERROR:"):
        level = "ERROR"
    elif isinstance(message, str) and message.startswith("WARNING:"):
        level = "WARN"
    _log(level, message, **fields)


def parse_args():
    p = argparse.ArgumentParser()
    p.add_argument("--input", required=True)
    p.add_argument("--output", required=True)
    return p.parse_args()


def build_context(seg):
    lines = []

    seg_start = float(seg.get("start_time", 0.0) or 0.0)
    seg_end = float(seg.get("end_time", 0.0) or 0.0)
    seg_dur = seg_end - seg_start if seg_end >= seg_start else 0.0
    lines.append(
        "Current segment time: %.1fs to %.1fs (%.1fs duration)" % (seg_start, seg_end, seg_dur)
    )

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
    history = seg.get("history") or {}
    persistent = history.get("persistent_tracks") or []

    if tracks:
        lines.append(
            "Tracks overlapping this segment "
            "(label, track_index, absolute start, absolute end, duration, continuity):"
        )

        # Build quick lookup for persistent
        pers_set = {
            (str(p.get("label")), int(p.get("track_index", -1)))
            for p in persistent
            if p.get("label") is not None
        }
        # Need also start time for persistent display
        pers_start_map = {
            (str(p.get("label")), int(p.get("track_index", -1))): float(p.get("start", 0.0) or 0.0)
            for p in persistent
        }

        for t in tracks:
            label = str(t.get("label", "?"))
            idx = int(t.get("track_index", -1))
            s = float(t.get("start", 0.0) or 0.0)
            e = float(t.get("end", 0.0) or 0.0)
            dur = e - s if e >= s else 0.0
            continuity = "new in this segment"
            key = (label, idx)
            if key in pers_set:
                # check persistent start < seg_start
                ps = pers_start_map.get(key, s)
                if ps < seg_start - 1e-6:
                    continuity = "continuing from %.1fs" % ps
            lines.append(
                "- %s track %d, %.1fs to %.1fs (%.1fs), %s"
                % (label, idx, s, e, dur, continuity)
            )
    else:
        lines.append(
            "Tracks overlapping this segment: none"
        )

    events = seg.get("events", []) or []

    if events:
        lines.append(
            "Events in this segment "
            "(type, label, absolute start, end):"
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

    # Temporal history: prior segments, persistent tracks, prior events
    if history:
        prior_segments = history.get("prior_segments") or []
        if prior_segments:
            lines.append(
                "Temporal history: %d prior segment(s) before %.1fs:" % (len(prior_segments), seg_start)
            )
            for ps in prior_segments:
                p_idx = ps.get("segment_index", "?")
                p_s = float(ps.get("start_time", 0.0) or 0.0)
                p_e = float(ps.get("end_time", 0.0) or 0.0)
                p_labels = ps.get("labels") or []
                lab_str = ", ".join(str(x) for x in p_labels) if p_labels else "none"
                p_tracks = ps.get("tracks") or []
                p_events = ps.get("events") or []
                lines.append(
                    "- Prior segment %s (%.1fs-%.1fs): labels [%s], %d track(s), %d event(s)"
                    % (p_idx, p_s, p_e, lab_str, len(p_tracks), len(p_events))
                )
                for pt in p_tracks:
                    lines.append(
                        "  * track %s %s: %.1fs-%.1fs"
                        % (
                            pt.get("label", "?"),
                            pt.get("track_index", "?"),
                            float(pt.get("start", 0.0) or 0.0),
                            float(pt.get("end", 0.0) or 0.0),
                        )
                    )
                for pe in p_events:
                    pe_end = pe.get("end")
                    pe_end_s = "none" if pe_end is None else "%.1fs" % float(pe_end)
                    lines.append(
                        "  * event %s %s: %.1fs to %s"
                        % (
                            pe.get("event_type", "?"),
                            pe.get("label", "?"),
                            float(pe.get("start", 0.0) or 0.0),
                            pe_end_s,
                        )
                    )
        else:
            lines.append(
                "Temporal history: no prior segments (this is the first segment)"
            )

        if persistent:
            lines.append(
                "Persistent tracks active before this segment (%d):" % len(persistent)
            )
            for pt in persistent:
                lines.append(
                    "- %s track %s, %.1fs to %.1fs (%.1fs)"
                    % (
                        pt.get("label", "?"),
                        pt.get("track_index", "?"),
                        float(pt.get("start", 0.0) or 0.0),
                        float(pt.get("end", 0.0) or 0.0),
                        float(pt.get("end", 0.0) or 0.0) - float(pt.get("start", 0.0) or 0.0),
                    )
                )
        else:
            lines.append(
                "Persistent tracks before this segment: none"
            )

        prior_events = history.get("prior_events") or []
        if prior_events:
            lines.append(
                "Prior events before this segment (%d):" % len(prior_events)
            )
            for pe in prior_events:
                pe_end = pe.get("end")
                pe_end_s = "none" if pe_end is None else "%.1fs" % float(pe_end)
                lines.append(
                    "- %s %s: %.1fs to %s"
                    % (
                        pe.get("event_type", "?"),
                        pe.get("label", "?"),
                        float(pe.get("start", 0.0) or 0.0),
                        pe_end_s,
                    )
                )
        else:
            lines.append(
                "Prior events before this segment: none"
            )

    append_block = (
        "\nStructured context (compact):\n"
        + "\n".join(lines)
    )

    return INSTRUCTION + append_block


def log_gpu_state(torch):
    try:
        cuda_available = torch.cuda.is_available()

        _log("INFO", f"CUDA available: {cuda_available}", cuda_available=cuda_available)

        if not cuda_available:
            return

        device_count = torch.cuda.device_count()

        _log("INFO", f"CUDA device count: {device_count}", device_count=device_count)

        for i in range(device_count):
            name = torch.cuda.get_device_name(i)
            props = torch.cuda.get_device_properties(i)

            _log("INFO", f"GPU {i}: {name}", gpu_index=i, gpu_name=name)

            _log(
                "INFO",
                f"GPU {i} total memory: {props.total_memory / (1024 ** 3):.2f} GB",
                gpu_index=i,
                total_memory_gb=round(props.total_memory / (1024 ** 3), 2),
            )

            allocated = torch.cuda.memory_allocated(i)
            reserved = torch.cuda.memory_reserved(i)

            _log(
                "INFO",
                f"GPU {i} memory: {allocated / (1024 ** 3):.2f} GB allocated, {reserved / (1024 ** 3):.2f} GB reserved",
                gpu_index=i,
                allocated_gb=round(allocated / (1024 ** 3), 2),
                reserved_gb=round(reserved / (1024 ** 3), 2),
            )

    except Exception as e:
        _log("WARN", f"Failed to inspect GPU state: {e}", error=str(e))


def main():
    total_start = time.time()

    args = parse_args()

    tmp_root = tempfile.mkdtemp(
        prefix="recall-vision-"
    )

    _log("INFO", "========================================")
    _log("INFO", "SmolVLM2-500M-Video-Instruct vision worker starting", model_name=MODEL_NAME, model_path=MODEL_PATH, model_version=MODEL_VERSION)
    _log("INFO", "========================================")

    _log("INFO", f"Python executable: {sys.executable}", python_executable=sys.executable)
    _log("INFO", f"Python version: {sys.version.split()[0]}", python_version=sys.version.split()[0])
    _log("INFO", f"Working directory: {os.getcwd()}", cwd=os.getcwd())
    _log("INFO", f"Input: {args.input}", input_path=args.input)
    _log("INFO", f"Output: {args.output}", output_path=args.output)
    _log("INFO", f"Model name: {MODEL_NAME}", model_name=MODEL_NAME)
    _log("INFO", f"Model path: {MODEL_PATH}", model_path=MODEL_PATH)
    _log("INFO", f"Model version: {MODEL_VERSION}", model_version=MODEL_VERSION)

    # ------------------------------------------------------------
    # Check model directory
    # ------------------------------------------------------------

    _log("INFO", "Checking model directory...", model_path=MODEL_PATH)

    if not os.path.exists(MODEL_PATH):
        _log("ERROR", f"model directory does not exist: {MODEL_PATH}", model_path=MODEL_PATH)

        shutil.rmtree(
            tmp_root,
            ignore_errors=True,
        )

        return 3

    if not os.path.isdir(MODEL_PATH):
        _log("ERROR", f"model path is not a directory: {MODEL_PATH}", model_path=MODEL_PATH)

        shutil.rmtree(
            tmp_root,
            ignore_errors=True,
        )

        return 3

    try:
        model_files = os.listdir(MODEL_PATH)

        _log("INFO", f"Model directory exists: {len(model_files)} entries", file_count=len(model_files), model_path=MODEL_PATH)

        for filename in model_files[:20]:
            _log("DEBUG", f"Model file: {filename}", filename=filename)

    except Exception as e:
        _log("WARN", f"failed to inspect model directory: {e}", error=str(e))

    # ------------------------------------------------------------
    # Read input
    # ------------------------------------------------------------

    _log("INFO", "Reading input JSON...", input_path=args.input)
    read_start = time.time()

    try:
        with open(args.input) as f:
            data = json.load(f)

    except Exception as e:
        _log("ERROR", f"failed to read input: {e}", error=str(e), input_path=args.input)

        shutil.rmtree(
            tmp_root,
            ignore_errors=True,
        )

        return 2

    read_ms = (time.time() - read_start) * 1000
    video_id = data.get("video_id", "")
    segments = data.get("segments", [])

    if not video_id or not isinstance(segments, list):
        _log("ERROR", "video_id and segments are required", video_id=video_id)

        shutil.rmtree(
            tmp_root,
            ignore_errors=True,
        )

        return 2

    _log("INFO", f"Video ID: {video_id}", video_id=video_id, duration_ms=round(read_ms, 2))
    _log("INFO", f"Segments: {len(segments)}", segment_count=len(segments), input_read_ms=round(read_ms, 2))

    # ------------------------------------------------------------
    # PIL
    # ------------------------------------------------------------

    _log("INFO", "Importing PIL...")
    pil_start = time.time()

    try:
        from PIL import Image

        _log("INFO", "PIL import OK", duration_ms=round((time.time() - pil_start) * 1000, 2))

    except Exception as e:
        _log("ERROR", f"PIL import failed: {e}", error=str(e), duration_ms=round((time.time() - pil_start) * 1000, 2))

        shutil.rmtree(
            tmp_root,
            ignore_errors=True,
        )

        return 3

    # ------------------------------------------------------------
    # Torch
    # ------------------------------------------------------------

    _log("INFO", "Importing torch...")
    torch_start = time.time()

    try:
        import torch

        _log("INFO", f"Torch import OK: {torch.__version__}", torch_version=torch.__version__, duration_ms=round((time.time() - torch_start) * 1000, 2))

        log_gpu_state(torch)

    except Exception as e:
        _log("ERROR", f"torch import failed: {e}", error=str(e), duration_ms=round((time.time() - torch_start) * 1000, 2))

        shutil.rmtree(
            tmp_root,
            ignore_errors=True,
        )

        return 3

    # ------------------------------------------------------------
    # Transformers
    # ------------------------------------------------------------

    _log("INFO", "Importing transformers...")
    tf_start = time.time()

    try:
        import transformers

        _log("INFO", f"Transformers import OK: {transformers.__version__}", transformers_version=transformers.__version__, duration_ms=round((time.time() - tf_start) * 1000, 2))

        from transformers import (
            AutoProcessor,
            AutoModelForImageTextToText,
        )

        _log("DEBUG", "AutoProcessor import OK")
        _log("DEBUG", "AutoModelForImageTextToText import OK")

    except Exception as e:
        _log("ERROR", f"transformers import failed: {e}", error=str(e), duration_ms=round((time.time() - tf_start) * 1000, 2))

        shutil.rmtree(
            tmp_root,
            ignore_errors=True,
        )

        return 3

    # ------------------------------------------------------------
    # Model load
    # ------------------------------------------------------------

    _log("INFO", "========================================")
    _log("INFO", "STARTING MODEL LOAD", model_path=MODEL_PATH)
    _log("INFO", "========================================")

    model_start = time.time()

    try:
        _log("INFO", f"Loading processor from: {MODEL_PATH}", model_path=MODEL_PATH)

        processor_start = time.time()

        processor = AutoProcessor.from_pretrained(
            MODEL_PATH
        )

        proc_ms = (time.time() - processor_start) * 1000
        _log("INFO", f"Processor loaded in {proc_ms/1000:.2f}s", duration_ms=round(proc_ms, 2), model_path=MODEL_PATH)

        _log("INFO", f"Loading model from: {MODEL_PATH}", model_path=MODEL_PATH)
        _log("INFO", "Using CPU float32 inference", dtype="float32", device="cpu")

        model_load_start = time.time()
        model = AutoModelForImageTextToText.from_pretrained(
            MODEL_PATH,
            torch_dtype=torch.float32,
        )

        model.eval()
        model_ms = (time.time() - model_load_start) * 1000
        total_model_ms = (time.time() - model_start) * 1000

        _log("INFO", f"MODEL LOAD COMPLETE in {total_model_ms/1000:.2f}s", duration_ms=round(total_model_ms, 2), processor_ms=round(proc_ms, 2), model_ms=round(model_ms, 2))
        _log("INFO", f"Model device: {getattr(model, 'device', 'unknown')}", model_device=str(getattr(model, 'device', 'unknown')))
        log_gpu_state(torch)

    except Exception as e:
        _log("ERROR", f"failed to load model: {e}", error=str(e), duration_ms=round((time.time() - model_start) * 1000, 2))

        shutil.rmtree(
            tmp_root,
            ignore_errors=True,
        )

        return 3

    _log("INFO", "========================================")
    _log("INFO", "MODEL READY", total_duration_ms=round((time.time() - model_start) * 1000, 2))
    _log("INFO", "========================================")

    descriptions = []

    # ------------------------------------------------------------
    # Segments
    # ------------------------------------------------------------

    vlm_total_start = time.time()
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

            _log("INFO", "----------------------------------------")
            _log("INFO", f"Segment {segment_index}/{len(segments)}", segment_index=segment_index, total_segments=len(segments), segment_id=segment_id)
            _log("INFO", f"Segment ID: {segment_id}", segment_id=segment_id)
            _log("INFO", f"Frame count: {len(frames)}", segment_id=segment_id, frame_count=len(frames))

            if not segment_id or not frames:
                _log("ERROR", "invalid segment: id and frames required", segment_id=segment_id, segment_index=segment_index)

                shutil.rmtree(
                    tmp_root,
                    ignore_errors=True,
                )

                return 2

            # ----------------------------------------------------
            # Load frames
            # ----------------------------------------------------

            images = []
            frame_load_total_ms = 0

            for frame_index, fr in enumerate(
                frames,
                start=1,
            ):
                path = fr.get(
                    "path",
                    "",
                )

                if not path:
                    _log("ERROR", f"frame {frame_index} missing path", segment_id=segment_id, frame_index=frame_index)

                    shutil.rmtree(
                        tmp_root,
                        ignore_errors=True,
                    )

                    return 2

                _log("DEBUG", f"Loading frame {frame_index}/{len(frames)}: {path}", segment_id=segment_id, frame_index=frame_index, path=path)

                frame_start = time.time()

                try:
                    img = Image.open(
                        path
                    ).convert("RGB")

                    frame_ms = (time.time() - frame_start) * 1000
                    frame_load_total_ms += frame_ms
                    _log(
                        "INFO",
                        f"Frame loaded: {img.width}x{img.height} in {frame_ms/1000:.3f}s",
                        segment_id=segment_id,
                        frame_index=frame_index,
                        width=img.width,
                        height=img.height,
                        duration_ms=round(frame_ms, 2),
                    )

                except Exception as e:
                    _log("ERROR", f"failed to open frame {path}: {e}", segment_id=segment_id, frame_index=frame_index, path=path, error=str(e))

                    shutil.rmtree(
                        tmp_root,
                        ignore_errors=True,
                    )

                    return 2

                images.append(img)

            _log("INFO", f"All {len(images)} frames loaded", segment_id=segment_id, frame_count=len(images), total_frame_load_ms=round(frame_load_total_ms, 2))

            # ----------------------------------------------------
            # Prompt
            # ----------------------------------------------------

            _log("DEBUG", "Building prompt...", segment_id=segment_id)

            prompt_start = time.time()

            prompt = build_context(seg)

            prompt_ms = (time.time() - prompt_start) * 1000
            _log("INFO", f"Prompt built in {prompt_ms/1000:.3f}s", segment_id=segment_id, duration_ms=round(prompt_ms, 2), prompt_length=len(prompt))

            _log("DEBUG", f"Prompt length: {len(prompt)} characters", segment_id=segment_id, prompt_length=len(prompt))

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

            _log("DEBUG", "Applying chat template...", segment_id=segment_id)

            template_start = time.time()

            text = processor.apply_chat_template(
                messages,
                tokenize=False,
                add_generation_prompt=True,
            )

            template_ms = (time.time() - template_start) * 1000
            _log("INFO", f"Chat template complete in {template_ms/1000:.3f}s", segment_id=segment_id, duration_ms=round(template_ms, 2), formatted_prompt_length=len(text))

            # ----------------------------------------------------
            # Processor
            # ----------------------------------------------------

            _log("INFO", "STARTING IMAGE PROCESSING", segment_id=segment_id)

            processor_start = time.time()

            inputs = processor(
                text=[text],
                images=images,
                padding=True,
                return_tensors="pt",
            )

            proc_img_ms = (time.time() - processor_start) * 1000
            _log("INFO", f"IMAGE PROCESSING COMPLETE in {proc_img_ms/1000:.2f}s", segment_id=segment_id, duration_ms=round(proc_img_ms, 2))

            tensor_shapes = {}

            for key, value in inputs.items():
                if hasattr(value, "shape"):
                    tensor_shapes[key] = tuple(
                        value.shape
                    )

            _log("DEBUG", f"Input tensors: {tensor_shapes}", segment_id=segment_id, tensor_shapes=tensor_shapes)
            _log("INFO", "Moving inputs to model device...", segment_id=segment_id, model_device=str(getattr(model, 'device', 'unknown')))

            move_start = time.time()

            inputs = inputs.to(
                model.device
            )

            move_ms = (time.time() - move_start) * 1000
            _log("INFO", f"Inputs moved in {move_ms/1000:.3f}s", segment_id=segment_id, duration_ms=round(move_ms, 2))

            # ----------------------------------------------------
            # Generation
            # ----------------------------------------------------

            _log("INFO", "========================================")
            _log("INFO", f"STARTING GENERATION: {segment_id}", segment_id=segment_id, segment_index=segment_index)
            _log("INFO", "CPU inference may take several minutes.", segment_id=segment_id)
            _log("INFO", "========================================")

            generation_start = time.time()

            with torch.inference_mode():
                generated = model.generate(
                    **inputs,
                    max_new_tokens=MAX_OUTPUT_TOKENS,
                    do_sample=False,
                )

            generation_time = (
                time.time() - generation_start
            )
            generation_ms = generation_time * 1000

            _log("INFO", f"GENERATION COMPLETE in {generation_time:.2f}s", segment_id=segment_id, duration_ms=round(generation_ms, 2), generated_shape=str(tuple(generated.shape)))

            # ----------------------------------------------------
            # Decode
            # ----------------------------------------------------

            _log("DEBUG", "Decoding generated tokens...", segment_id=segment_id)
            decode_start = time.time()

            trimmed = generated[
                :,
                inputs.input_ids.shape[1]:
            ]

            desc = processor.batch_decode(
                trimmed,
                skip_special_tokens=True,
                clean_up_tokenization_spaces=False,
            )[0].strip()

            decode_ms = (time.time() - decode_start) * 1000

            if not desc:
                _log("ERROR", f"empty description for segment {segment_id}", segment_id=segment_id, duration_ms=round(decode_ms, 2))

                shutil.rmtree(
                    tmp_root,
                    ignore_errors=True,
                )

                return 3

            _log("INFO", f"Description generated ({len(desc)} chars):", segment_id=segment_id, description_length=len(desc), decode_ms=round(decode_ms, 2))
            _log("INFO", desc, segment_id=segment_id, description_length=len(desc))

            descriptions.append(
                {
                    "segment_id": segment_id,
                    "description": desc,
                    "model_name": MODEL_NAME,
                    "model_version": MODEL_VERSION,
                }
            )

            segment_ms = (time.time() - segment_start) * 1000
            _log("INFO", f"Segment {segment_id} COMPLETE in {segment_ms/1000:.2f}s", segment_id=segment_id, duration_ms=round(segment_ms, 2), segment_index=segment_index)

    except SystemExit:
        raise

    except Exception as e:
        _log("ERROR", f"inference failed: {e}", error=str(e))

        shutil.rmtree(
            tmp_root,
            ignore_errors=True,
        )

        return 3

    vlm_total_ms = (time.time() - vlm_total_start) * 1000

    # ------------------------------------------------------------
    # Write output
    # ------------------------------------------------------------

    try:
        _log("INFO", "Writing output JSON...", output_path=args.output)

        out = {
            "video_id": video_id,
            "descriptions": descriptions,
        }

        with open(args.output, "w") as f:
            json.dump(
                out,
                f,
            )

        _log("INFO", f"Output written: {args.output}", output_path=args.output)

    except Exception as e:
        _log("ERROR", f"failed to write output: {e}", error=str(e), output_path=args.output)

        shutil.rmtree(
            tmp_root,
            ignore_errors=True,
        )

        return 2

    shutil.rmtree(
        tmp_root,
        ignore_errors=True,
    )

    total_ms = (time.time() - total_start) * 1000
    _log("INFO", "========================================")
    _log("INFO", f"VISION WORKER COMPLETE in {total_ms/1000:.2f}s", total_duration_ms=round(total_ms, 2), vlm_duration_ms=round(vlm_total_ms, 2), model_load_ms=round((model_start and (time.time() - model_start)*1000) if 'model_start' in locals() else 0, 2), segment_count=len(descriptions))
    _log("INFO", f"Descriptions generated: {len(descriptions)}", description_count=len(descriptions), total_duration_ms=round(total_ms, 2))
    _log("INFO", "========================================")

    return 0


if __name__ == "__main__":
    sys.exit(main())
