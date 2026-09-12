#!/usr/bin/env python3
import argparse
import json
import os
import sys
import time

# Ensure unbuffered, line-buffered output
try:
    sys.stdout.reconfigure(line_buffering=True, write_through=True)
    sys.stderr.reconfigure(line_buffering=True, write_through=True)
except Exception:
    pass


def log(level, msg, **fields):
    """Production-grade JSON logger to stderr."""
    record = {
        "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "level": level,
        "component": "detector",
        "msg": msg,
    }
    if fields:
        record.update(fields)
    try:
        print(json.dumps(record), file=sys.stderr, flush=True)
    except Exception:
        print(f"[{record['timestamp']}] [{level}] [detector] {msg} {fields}", file=sys.stderr, flush=True)


def clamp(v):
    return max(0.0, min(1.0, v))


def parse_args():
    p = argparse.ArgumentParser()
    p.add_argument("--input", required=True)
    p.add_argument("--output", required=True)
    p.add_argument("--threshold", type=float, default=0.25)
    p.add_argument("--model", default="")
    return p.parse_args()


def load_model(model_path):
    """Load YOLO model with timed, structured logging."""
    load_start = time.time()
    log("INFO", "model_load_start", model_path=model_path if model_path else "yolov8n.pt")
    try:
        from ultralytics import YOLO

        mp = model_path if model_path else "yolov8n.pt"
        log("INFO", "model_loading", model_path=mp)
        model = YOLO(mp)
        duration_ms = (time.time() - load_start) * 1000
        log("INFO", "model_load_complete", model_path=mp, duration_ms=round(duration_ms, 2), model_type="ultralytics")
        return model, "ultralytics", duration_ms
    except Exception as e:
        duration_ms = (time.time() - load_start) * 1000
        log("ERROR", "model_load_failed", error=str(e), duration_ms=round(duration_ms, 2))
        print(f"ultralytics not available: {e}", file=sys.stderr)
        return None, None, duration_ms


def detect_with_yolo(model, frames, threshold):
    results = {}
    total_inference_ms = 0
    total_frames = len(frames)
    log("INFO", "detection_start", total_frames=total_frames, threshold=threshold)

    for idx, fr in enumerate(frames, start=1):
        fid = fr["frame_id"]
        path = fr["path"]
        w = fr.get("width", 640)
        h = fr.get("height", 480)
        frame_start = time.time()
        try:
            preds = model(path, verbose=False)
        except Exception as e:
            duration_ms = (time.time() - frame_start) * 1000
            log("ERROR", "inference_failed", frame_id=fid, frame_index=idx, path=path, error=str(e), duration_ms=round(duration_ms, 2))
            print(f"inference failed for {path}: {e}", file=sys.stderr)
            results[fid] = []
            continue

        inference_ms = (time.time() - frame_start) * 1000
        total_inference_ms += inference_ms

        dets = []
        det_count_before_filter = 0
        for r in preds:
            boxes = r.boxes
            if boxes is None:
                continue
            for box in boxes:
                det_count_before_filter += 1
                conf = float(box.conf.item()) if hasattr(box.conf, "item") else float(box.conf[0])
                if conf < threshold:
                    continue
                cls_id = int(box.cls.item()) if hasattr(box.cls, "item") else int(box.cls[0])
                label = r.names.get(cls_id, str(cls_id)) if hasattr(r, "names") else str(cls_id)
                xyxy = box.xyxy[0].tolist() if hasattr(box.xyxy[0], "tolist") else list(box.xyxy[0])
                x1, y1, x2, y2 = xyxy
                fw = w if w > 0 else 640
                fh = h if h > 0 else 480
                bx = clamp(x1 / fw)
                by = clamp(y1 / fh)
                bw = clamp((x2 - x1) / fw)
                bh = clamp((y2 - y1) / fh)
                if bx + bw > 1:
                    bw = 1 - bx
                if by + bh > 1:
                    bh = 1 - by
                if bw <= 0 or bh <= 0:
                    continue
                dets.append(
                    {
                        "label": label,
                        "confidence": conf,
                        "bbox_x": bx,
                        "bbox_y": by,
                        "bbox_width": bw,
                        "bbox_height": bh,
                    }
                )

        per_frame_avg = total_inference_ms / idx if idx > 0 else 0
        log(
            "INFO",
            "frame_detection_complete",
            frame_id=fid,
            frame_index=idx,
            total_frames=total_frames,
            detections_kept=len(dets),
            detections_raw=det_count_before_filter,
            duration_ms=round(inference_ms, 2),
            avg_ms_per_frame=round(per_frame_avg, 2),
        )
        results[fid] = dets

    avg_ms = total_inference_ms / total_frames if total_frames > 0 else 0
    log(
        "INFO",
        "detection_complete",
        total_frames=total_frames,
        total_inference_ms=round(total_inference_ms, 2),
        avg_ms_per_frame=round(avg_ms, 2),
    )
    return results


def main():
    total_start = time.time()
    args = parse_args()

    log("INFO", "detector_worker_start", input=args.input, output=args.output, threshold=args.threshold, model=args.model, pid=os.getpid())

    # Read input JSON with timing
    read_start = time.time()
    try:
        with open(args.input) as f:
            data = json.load(f)
    except Exception as e:
        log("ERROR", "failed_to_read_input", input_path=args.input, error=str(e))
        sys.exit(2)

    read_ms = (time.time() - read_start) * 1000
    frames = data.get("frames", [])
    log("INFO", "input_loaded", frames=len(frames), duration_ms=round(read_ms, 2), input_path=args.input)

    threshold = args.threshold

    # Model loading (timed inside load_model)
    model, kind, model_load_ms = load_model(args.model)
    if model is None:
        log("ERROR", "model_unavailable_failing", model_path=args.model)
        print(
            "FATAL: ultralytics not installed or model failed to load; failing loud (no synthetic fallback)",
            file=sys.stderr,
        )
        try:
            with open(args.output, "w") as outf:
                json.dump({}, outf)
        except Exception:
            pass
        sys.exit(2)

    # Detection
    detect_start = time.time()
    out = detect_with_yolo(model, frames, threshold)
    detect_ms = (time.time() - detect_start) * 1000

    # Aggregate stats
    total_dets = sum(len(v) for v in out.values())
    log("INFO", "detection_summary", total_detections=total_dets, detection_duration_ms=round(detect_ms, 2))

    # Write output
    write_start = time.time()
    try:
        with open(args.output, "w") as outf:
            json.dump(out, outf)
    except Exception as e:
        log("ERROR", "failed_to_write_output", output_path=args.output, error=str(e))
        sys.exit(2)

    write_ms = (time.time() - write_start) * 1000
    total_ms = (time.time() - total_start) * 1000

    log(
        "INFO",
        "detector_worker_complete",
        total_frames=len(frames),
        total_detections=total_dets,
        model_load_ms=round(model_load_ms, 2),
        detection_ms=round(detect_ms, 2),
        write_ms=round(write_ms, 2),
        total_duration_ms=round(total_ms, 2),
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
