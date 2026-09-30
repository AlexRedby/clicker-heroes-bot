package main

import (
	_ "embed"
	"fmt"
	"image"
	"math"
	"sort"

	"gocv.io/x/gocv"
)

//go:embed assets/orange-fish.png
var fishPNG []byte

type fishReference struct {
	width, height int
	points        []gocv.KeyPoint
	descriptors   gocv.Mat
}

type siftFishDetector struct {
	sift       gocv.SIFT
	matcher    gocv.BFMatcher
	references []fishReference
}

func newSIFTFishDetector() (*siftFishDetector, error) {
	fish, err := gocv.IMDecode(fishPNG, gocv.IMReadUnchanged)
	if err != nil {
		return nil, fmt.Errorf("decode fish image: %w", err)
	}
	defer fish.Close()
	if fish.Empty() || fish.Channels() != 4 {
		return nil, fmt.Errorf("fish image must have an alpha channel")
	}

	contrast, sigma := 0.001, 0.8
	detector := &siftFishDetector{
		sift:    gocv.NewSIFTWithParams(nil, nil, &contrast, nil, &sigma),
		matcher: gocv.NewBFMatcherWithParams(gocv.NormL2, false),
	}
	for _, height := range []int{50, 75, 200} {
		if err := detector.addReference(fish, height); err != nil {
			detector.Close()
			return nil, err
		}
	}
	return detector, nil
}

func (detector *siftFishDetector) addReference(fish gocv.Mat, height int) error {
	width := int(math.Round(float64(fish.Cols()) * float64(height) / float64(fish.Rows())))
	resized := gocv.NewMat()
	defer resized.Close()
	if err := gocv.Resize(fish, &resized, image.Pt(width, height), 0, 0, gocv.InterpolationArea); err != nil {
		return fmt.Errorf("resize fish reference: %w", err)
	}
	gray := gocv.NewMat()
	defer gray.Close()
	if err := gocv.CvtColor(resized, &gray, gocv.ColorBGRAToGray); err != nil {
		return fmt.Errorf("convert fish reference: %w", err)
	}
	alpha := gocv.NewMat()
	defer alpha.Close()
	if err := gocv.ExtractChannel(resized, &alpha, 3); err != nil {
		return fmt.Errorf("extract fish alpha: %w", err)
	}
	mask := gocv.NewMat()
	defer mask.Close()
	gocv.Threshold(alpha, &mask, 180, 255, gocv.ThresholdBinary)
	points, descriptors := detector.sift.DetectAndCompute(gray, mask)
	detector.references = append(detector.references, fishReference{width, height, points, descriptors})
	return nil
}

func (detector *siftFishDetector) Close() {
	for _, reference := range detector.references {
		reference.descriptors.Close()
	}
	detector.matcher.Close()
	detector.sift.Close()
}

func (detector *siftFishDetector) Find(screen image.Image) (image.Point, bool, error) {
	if screen == nil {
		return image.Point{}, false, fmt.Errorf("screen image is nil")
	}
	color, err := gocv.ImageToMatRGB(screen)
	if err != nil {
		return image.Point{}, false, fmt.Errorf("convert screen image: %w", err)
	}
	defer color.Close()
	gray := gocv.NewMat()
	defer gray.Close()
	if err := gocv.CvtColor(color, &gray, gocv.ColorBGRToGray); err != nil {
		return image.Point{}, false, fmt.Errorf("convert screen to grayscale: %w", err)
	}
	mask := gocv.NewMat()
	defer mask.Close()
	scenePoints, sceneDescriptors := detector.sift.DetectAndCompute(gray, mask)
	defer sceneDescriptors.Close()
	if sceneDescriptors.Empty() {
		return image.Point{}, false, nil
	}

	bounds := screen.Bounds()
	var bestPoint image.Point
	bestInliers := 0
	for _, reference := range detector.references {
		if reference.descriptors.Empty() {
			continue
		}
		point, inliers := detector.matchReference(reference, scenePoints, sceneDescriptors, bounds.Size())
		if inliers > bestInliers {
			bestPoint = point.Add(bounds.Min)
			bestInliers = inliers
		}
	}
	return bestPoint, bestInliers > 0, nil
}

func (detector *siftFishDetector) matchReference(reference fishReference, scenePoints []gocv.KeyPoint, sceneDescriptors gocv.Mat, screenSize image.Point) (image.Point, int) {
	pairs := detector.matcher.KnnMatch(reference.descriptors, sceneDescriptors, 2)
	matches := make([]gocv.DMatch, 0, len(pairs))
	for _, pair := range pairs {
		if len(pair) == 2 && pair[0].Distance < 0.85*pair[1].Distance {
			matches = append(matches, pair[0])
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Distance < matches[j].Distance })

	seen := make(map[image.Point]bool, len(matches))
	source := make([]gocv.Point2f, 0, len(matches))
	target := make([]gocv.Point2f, 0, len(matches))
	for _, match := range matches {
		from := reference.points[match.QueryIdx]
		to := scenePoints[match.TrainIdx]
		pixel := image.Pt(int(math.Round(to.X)), int(math.Round(to.Y)))
		if seen[pixel] {
			continue
		}
		seen[pixel] = true
		source = append(source, gocv.Point2f{X: float32(from.X), Y: float32(from.Y)})
		target = append(target, gocv.Point2f{X: float32(to.X), Y: float32(to.Y)})
	}
	if len(source) < 4 {
		return image.Point{}, 0
	}

	from := gocv.NewPoint2fVectorFromPoints(source)
	defer from.Close()
	to := gocv.NewPoint2fVectorFromPoints(target)
	defer to.Close()
	inlierMask := gocv.NewMat()
	defer inlierMask.Close()
	transform := gocv.EstimateAffinePartial2DWithParams(from, to, inlierMask, int(gocv.HomographyMethodRANSAC), 3, 2000, 0.99, 10)
	defer transform.Close()
	if transform.Empty() {
		return image.Point{}, 0
	}
	inliers := gocv.CountNonZero(inlierMask)
	if inliers < 4 {
		return image.Point{}, 0
	}

	a, b, tx := transform.GetDoubleAt(0, 0), transform.GetDoubleAt(0, 1), transform.GetDoubleAt(0, 2)
	c, d, ty := transform.GetDoubleAt(1, 0), transform.GetDoubleAt(1, 1), transform.GetDoubleAt(1, 2)
	height := float64(reference.height) * math.Hypot(a, c)
	x := a*float64(reference.width)/2 + b*float64(reference.height)/2 + tx
	y := c*float64(reference.width)/2 + d*float64(reference.height)/2 + ty
	center, inside := fishCenter(x, y, screenSize)
	if height < 50 || height > math.Min(400, float64(screenSize.Y)/4) ||
		!inside {
		return image.Point{}, 0
	}
	return center, inliers
}

func fishCenter(x, y float64, size image.Point) (image.Point, bool) {
	if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
		return image.Point{}, false
	}
	center := image.Pt(int(math.Round(x)), int(math.Round(y)))
	return center, center.In(image.Rectangle{Max: size})
}
