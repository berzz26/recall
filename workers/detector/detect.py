#!/usr/bin/env python3
import argparse
import json
import os
import sys

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
    try:
        from ultralytics import YOLO
        mp = model_path if model_path else "yolov8n.pt"
        model = YOLO(mp)
        return model, "ultralytics"
    except Exception as e:
        print(f"ultralytics not available: {e}", file=sys.stderr)
        return None, None

def detect_with_yolo(model, frames, threshold):
    results = {}
    for fr in frames:
        fid = fr["frame_id"]
        path = fr["path"]
        w = fr.get("width", 640)
        h = fr.get("height", 480)
        try:
            preds = model(path, verbose=False)
        except Exception as e:
            print(f"inference failed for {path}: {e}", file=sys.stderr)
            results[fid] = []
            continue
        dets = []
        for r in preds:
            boxes = r.boxes
            if boxes is None:
                continue
            for box in boxes:
                conf = float(box.conf.item()) if hasattr(box.conf, 'item') else float(box.conf[0])
                if conf < threshold:
                    continue
                cls_id = int(box.cls.item()) if hasattr(box.cls, 'item') else int(box.cls[0])
                label = r.names.get(cls_id, str(cls_id)) if hasattr(r, 'names') else str(cls_id)
                xyxy = box.xyxy[0].tolist() if hasattr(box.xyxy[0], 'tolist') else list(box.xyxy[0])
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
                dets.append({"label": label, "confidence": conf, "bbox_x": bx, "bbox_y": by, "bbox_width": bw, "bbox_height": bh})
        results[fid] = dets
    return results

def main():
    args = parse_args()
    with open(args.input) as f:
        data = json.load(f)
    frames = data.get("frames", [])
    threshold = args.threshold
    model, kind = load_model(args.model)
    if model is None:
        print("FATAL: ultralytics not installed or model failed to load; failing loud (no synthetic fallback)", file=sys.stderr)
        # Do not write fake detections - let Go mark FAILED
        # Write empty output for debugging but exit non-zero
        try:
            with open(args.output, "w") as outf:
                json.dump({}, outf)
        except:
            pass
        sys.exit(2)
    out = detect_with_yolo(model, frames, threshold)
    with open(args.output, "w") as outf:
        json.dump(out, outf)
    return 0

if __name__ == "__main__":
    sys.exit(main())
