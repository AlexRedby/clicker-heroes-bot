"""Compare rotation and scale tolerant feature matchers on fish fixtures."""

from pathlib import Path
from statistics import median
from time import perf_counter

import cv2
import numpy as np


ROOT = Path(__file__).resolve().parents[1]
FISH = cv2.imread(str(ROOT / "assets/orange-fish.png"), cv2.IMREAD_UNCHANGED)
FIXTURES = {
    "no-fish": None,
    "upright": (208, 258),
    "tilted": (883, 79),
    "upside-down": (733, 245),
    "small-bilinear": (831, 81),
}


def small_bilinear_fixture():
    screen = cv2.imread(str(ROOT / "testdata/no-fish-game-screen.jpg"))
    height = 50
    width = round(FISH.shape[1] * height / FISH.shape[0])
    fish = cv2.resize(FISH, (width, height), interpolation=cv2.INTER_AREA)
    transform = cv2.getRotationMatrix2D((width / 2, height / 2), 45, 1)
    rotated_width = round((width + height) / np.sqrt(2))
    rotated_height = rotated_width
    transform[0, 2] += (rotated_width - width) / 2
    transform[1, 2] += (rotated_height - height) / 2
    fish = cv2.warpAffine(
        fish, transform, (rotated_width, rotated_height), flags=cv2.INTER_LINEAR
    )
    x, y = 800, 50
    alpha = fish[:, :, 3:4].astype(float) / 255
    patch = screen[y : y + rotated_height, x : x + rotated_width]
    patch[:] = (fish[:, :, :3] * alpha + patch * (1 - alpha)).astype(np.uint8)
    return screen


def make_matcher(name):
    if name == "SIFT":
        detector = cv2.SIFT_create(contrastThreshold=0.001, sigma=0.8)
        norm = cv2.NORM_L2
    else:
        detector = cv2.ORB_create(
            nfeatures=4000, fastThreshold=5, edgeThreshold=5, patchSize=21
        )
        norm = cv2.NORM_HAMMING

    templates = []
    heights = (50, 75, 200) if name == "SIFT" else (75, 200)
    for height in heights:
        width = round(FISH.shape[1] * height / FISH.shape[0])
        fish = cv2.resize(FISH, (width, height), interpolation=cv2.INTER_AREA)
        gray = cv2.cvtColor(fish[:, :, :3], cv2.COLOR_BGR2GRAY)
        mask = np.where(fish[:, :, 3] > 180, 255, 0).astype(np.uint8)
        points, descriptors = detector.detectAndCompute(gray, mask)
        templates.append((width, height, points, descriptors))
    return detector, cv2.BFMatcher(norm), templates


def find_fish(screen, matcher):
    detector, bf, templates = matcher
    gray = cv2.cvtColor(screen, cv2.COLOR_BGR2GRAY)
    scene_points, scene_descriptors = detector.detectAndCompute(gray, None)
    if scene_descriptors is None:
        return None, 0

    best_point, best_inliers = None, 0
    for width, height, points, descriptors in templates:
        if descriptors is None:
            continue
        pairs = bf.knnMatch(descriptors, scene_descriptors, k=2)
        matches = [a for a, b in pairs if a.distance < 0.85 * b.distance]
        matches.sort(key=lambda match: match.distance)
        unique = []
        used_scene_pixels = set()
        for match in matches:
            pixel = tuple(round(v) for v in scene_points[match.trainIdx].pt)
            if pixel not in used_scene_pixels:
                unique.append(match)
                used_scene_pixels.add(pixel)
        if len(unique) < 4:
            continue

        source = np.float32([points[match.queryIdx].pt for match in unique])
        target = np.float32([scene_points[match.trainIdx].pt for match in unique])
        transform, mask = cv2.estimateAffinePartial2D(
            source, target, method=cv2.RANSAC, ransacReprojThreshold=3
        )
        if transform is None:
            continue
        inliers = int(mask.sum())
        scale = np.linalg.norm(transform[:, 0])
        center = transform @ np.array([width / 2, height / 2, 1])
        if (
            inliers >= 4
            and 50 <= height * scale <= min(400, screen.shape[0] / 4)
            and 0 <= center[0] < screen.shape[1]
            and 0 <= center[1] < screen.shape[0]
            and inliers > best_inliers
        ):
            best_point = tuple(round(v) for v in center)
            best_inliers = inliers
    return best_point, best_inliers


def main():
    print(f"OpenCV {cv2.__version__}")
    for name in ("SIFT", "ORB"):
        matcher = make_matcher(name)
        print(name, "template keypoints", [len(t[2]) for t in matcher[2]])
        for fixture, expected in FIXTURES.items():
            screen = (
                small_bilinear_fixture()
                if fixture == "small-bilinear"
                else cv2.imread(
                    str(ROOT / "artifacts/gocv-comparison" / f"{fixture}.png")
                )
            )
            if screen is None:
                raise SystemExit(f"missing fixture: {fixture}")
            times = []
            for _ in range(5):
                start = perf_counter()
                point, inliers = find_fish(screen, matcher)
                times.append(perf_counter() - start)
            hit = point is not None and expected is not None and np.linalg.norm(
                np.subtract(point, expected)
            ) <= 15
            correct = (point is None) if expected is None else hit
            print(
                f"  {fixture}: correct={correct} point={point} "
                f"inliers={inliers} median={median(times):.3f}s"
            )
            if name == "SIFT" and not correct:
                raise AssertionError(f"SIFT failed on {fixture}")


if __name__ == "__main__":
    main()
