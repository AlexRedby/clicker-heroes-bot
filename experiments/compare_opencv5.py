from pathlib import Path
from time import perf_counter
import math
import sys

import cv2
import numpy as np


THRESHOLD = 0.97
ROOT = Path(__file__).resolve().parents[1]
FISH = cv2.imread(str(ROOT / "assets/orange-fish.png"), cv2.IMREAD_UNCHANGED)


def find_fish(screen):
    screen_height, screen_width = screen.shape[:2]
    source_height, source_width = FISH.shape[:2]
    best_score, best_point = 0.0, None
    for degrees in range(0, 360, 5):
        radians = math.radians(degrees)
        cosine, sine = abs(math.cos(radians)), abs(math.sin(radians))
        height = max(50, screen_height // 30)
        while height <= min(400, screen_height // 4):
            width = height * source_width // source_height
            rotated_width = math.ceil(width * cosine + height * sine)
            rotated_height = math.ceil(width * sine + height * cosine)
            if rotated_width <= screen_width and rotated_height <= screen_height:
                resized = cv2.resize(FISH, (width, height), interpolation=cv2.INTER_NEAREST)
                matrix = cv2.getRotationMatrix2D((width / 2, height / 2), -degrees, 1)
                matrix[0, 2] += (rotated_width - width) / 2
                matrix[1, 2] += (rotated_height - height) / 2
                template = cv2.warpAffine(
                    resized,
                    matrix,
                    (rotated_width, rotated_height),
                    flags=cv2.INTER_NEAREST,
                )
                mask = (template[:, :, 3] >= 224).astype(np.uint8)
                result = cv2.matchTemplate(
                    screen,
                    template[:, :, :3],
                    cv2.TM_CCORR_NORMED,
                    mask=mask,
                )
                _, score, _, location = cv2.minMaxLoc(result)
                if score > best_score:
                    best_score = score
                    best_point = (
                        location[0] + rotated_width // 2,
                        location[1] + rotated_height // 2,
                    )
                if best_score >= THRESHOLD:
                    return best_point, best_score
            height += max(2, height // 12)
    return best_point, best_score


def main():
    print(f"OpenCV {cv2.__version__}")
    names = sys.argv[1:] or ("no-fish", "upright", "tilted", "upside-down")
    for name in names:
        path = ROOT / "artifacts/gocv-comparison" / f"{name}.png"
        screen = cv2.imread(str(path), cv2.IMREAD_COLOR)
        if screen is None:
            raise SystemExit(f"missing fixture: {path}")
        start = perf_counter()
        point, score = find_fish(screen)
        elapsed = perf_counter() - start
        print(f"{name}: found={score >= THRESHOLD} point={point} score={score:.5f} time={elapsed:.3f}s")


if __name__ == "__main__":
    main()
