package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

//Frame is base object to hold frame info
// type Frame struct {
// 	Path    string
// 	Average color.RGBA
// 	Median  color.RGBA
// }

//RGB convenience type
type RGB [3]uint8

//R -> Red Component
func (c RGB) R() uint8 {
	return c[0]
}

//G -> Green Component
func (c RGB) G() uint8 {
	return c[1]
}

//B -> Blue Component
func (c RGB) B() uint8 {
	return c[2]
}

//RGBA gets the color.RGBA version
func (c RGB) RGBA() color.RGBA {
	return color.RGBA{c[0], c[1], c[2], 255}
}

//Hex gets the color.Hex version
func (c RGB) Hex() string {
	return fmt.Sprintf("#%02X%02X%02X", c[0], c[1], c[2])
}

//Uint32 gets the color.Hex version
func (c RGB) Uint32() uint32 {
	return ((uint32(c[0]) << 16) | (uint32(c[1]) << 8) | (uint32(c[2]) << 0))
}

type HSL struct {
	H, S, L float64
}

func (c RGB) ToHSL() HSL {
	var h, s, l float64

	r := float64(c.R())
	g := float64(c.G())
	b := float64(c.B())

	max := math.Max(math.Max(r, g), b)
	min := math.Min(math.Min(r, g), b)

	// Luminosity is the average of the max and min rgb color intensities.
	l = (max + min) / 2

	// saturation
	delta := max - min
	if delta == 0 {
		// it's gray
		return HSL{0, 0, l}
	}

	// it's not gray
	if l < 0.5 {
		s = delta / (max + min)
	} else {
		s = delta / (2 - max - min)
	}

	// hue
	r2 := (((max - r) / 6) + (delta / 2)) / delta
	g2 := (((max - g) / 6) + (delta / 2)) / delta
	b2 := (((max - b) / 6) + (delta / 2)) / delta
	switch {
	case r == max:
		h = b2 - g2
	case g == max:
		h = (1.0 / 3.0) + r2 - b2
	case b == max:
		h = (2.0 / 3.0) + g2 - r2
	}

	// fix wraparounds
	switch {
	case h < 0:
		h += 1
	case h > 1:
		h -= 1
	}

	return HSL{h, s, l}
}

//RGBList used for sorting
type RGBList []RGB

//Len for sort
func (l RGBList) Len() int {
	return len(l)
}

//Swap for sort
func (l RGBList) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

//Less for sort
func (l RGBList) Less(i, j int) bool {
	// return l[i].Uint32() < l[j].Uint32()
	return l[i].ToHSL().H < l[j].ToHSL().H
}

//Frame is base object to hold frame info
type Frame struct {
	Path    string
	Average RGB
	Median  RGB
	Mode    RGBList
}

type jsonFrame struct {
	Path    string
	Average string
	Median  string
	Mode    []string
}

func (f Frame) toJSONFrame() jsonFrame {
	var mode = make([]string, len(f.Mode))
	for i, m := range f.Mode {
		mode[i] = m.Hex()
	}
	return jsonFrame{f.Path, f.Average.Hex(), f.Median.Hex(), mode}
}

func openImage(filename string) image.Image {
	file, err := os.Open(filename)

	if err != nil {
		fmt.Println("Error: File could not be opened")
		os.Exit(1)
	}

	defer file.Close()

	img, _, err := image.Decode(file)

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	return img
}

// Pixel loading thanks to https://stackoverflow.com/questions/33186783/get-a-pixel-array-from-from-golang-image-image
// Get the bi-dimensional pixel array
func getPixels(filename string) (RGBList, error) {
	img := openImage(filename)

	bounds := img.Bounds()
	w, h := bounds.Max.X, bounds.Max.Y

	var pixels RGBList
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			pixels = append(pixels, rgbaToPixel(img.At(x, y).RGBA()))
		}
	}

	return pixels, nil
}

// img.At(x, y).RGBA() returns four uint32 values; we want a Pixel
func rgbaToPixel(r uint32, g uint32, b uint32, a uint32) RGB {
	return RGB{uint8(r / 257), uint8(g / 257), uint8(b / 257)}
}

func pipeReader(prefix string, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		fmt.Println(prefix + " > " + scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
}

func ffmpeg(opts ...string) {
	cmd := exec.Command("ffmpeg", opts...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error creating StdoutPipe for Cmd", err)
		os.Exit(1)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error creating StderrPipe for Cmd", err)
		os.Exit(1)
	}

	go pipeReader("ffmpeg", stdout)
	go pipeReader("ffmpeg", stderr)

	err = cmd.Start()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error starting ffmpeg", err)
		os.Exit(1)
	}

	err = cmd.Wait()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error waiting for ffmpeg", err)
		os.Exit(1)
	}
}

func isDir(dir string) (bool, error) {
	src, err := os.Stat(dir)

	if os.IsNotExist(err) {
		return false, nil
	}

	if src.Mode().IsRegular() {
		return false, errors.New("ERROR: " + dir + " already exists as a file!")
	}

	return true, nil
}

func createThumbs(input string, frameDir string) {
	res, err := isDir(frameDir)
	if !res {
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		errDir := os.MkdirAll(frameDir, 0755)
		if errDir != nil {
			fmt.Println(errDir)
			os.Exit(1)
		}
	}

	files, err := ioutil.ReadDir(frameDir)

	if err != nil {
		log.Fatal(err)
	}

	if len(files) > 0 {
		fmt.Println("Frames already found in " + frameDir + "\nSkipping thumbnail generation.")
		return
	}

	outFormat := frameDir + "img%06d.png"
	filter := fmt.Sprintf("fps=%s,scale=-2:%d", framerate, thumbHeight)
	opts := []string{"-progress", "pipe:1", "-i", input, "-vf", filter, outFormat}
	ffmpeg(opts...)
}

func getAverage(pixels RGBList) RGB {
	var r, g, b float64

	total := len(pixels)

	result := RGB{0, 0, 0}

	if avgSqr { //squared algorithm
		// https://sighack.com/post/averaging-rgb-colors-the-right-way?fbclid=IwAR3T1vH62sG1U1JuoSgOJ5-7XqtqekHKmp_Ebw6JwXczteQVkOdgpW5T4Sw
		for _, p := range pixels {
			r += math.Pow(float64(p.R()), 2)
			g += math.Pow(float64(p.G()), 2)
			b += math.Pow(float64(p.B()), 2)
		}

		result[0] = uint8(math.Sqrt(r / float64(total)))
		result[1] = uint8(math.Sqrt(g / float64(total)))
		result[2] = uint8(math.Sqrt(b / float64(total)))
	} else {
		for _, p := range pixels {
			r += float64(p.R())
			g += float64(p.G())
			b += float64(p.B())
		}

		result[0] = uint8(math.Floor(r / float64(total)))
		result[1] = uint8(math.Floor(g / float64(total)))
		result[2] = uint8(math.Floor(b / float64(total)))
	}

	return result
}

func getMedian(pixels RGBList) RGB {
	list := make(RGBList, len(pixels))
	copy(list, pixels)
	sort.Sort(list)

	return list[len(list)/2]
}

type colorCount struct {
	Key   string
	Value int
	Color RGB
}

type colorCountList []colorCount

func (p colorCountList) Len() int           { return len(p) }
func (p colorCountList) Less(i, j int) bool { return p[i].Value < p[j].Value }
func (p colorCountList) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func getMode(pixels RGBList) RGBList {
	var m = make(map[string]colorCount)

	for _, p := range pixels {
		k := p.Hex()
		if _, ok := m[k]; !ok {
			m[k] = colorCount{k, 0, p}
		}

		cc := m[k]
		cc.Value++
		m[k] = cc
	}

	counts := make(colorCountList, len(m))
	i := 0
	for _, v := range m {
		counts[i] = v
		i++
	}

	sort.Sort(counts)
	sort.Sort(sort.Reverse(counts))

	result := make(RGBList, genMode)
	for i := 0; i < len(counts) && i < genMode; i++ {
		result[i] = counts[i].Color
	}
	sort.Sort(result)
	return result
}

func processFrames(frameDir string) []Frame {
	fmt.Println("Generating average data for frames...")
	files, err := ioutil.ReadDir(frameDir)

	if err != nil {
		log.Fatal(err)
	}

	var result []Frame

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		fmt.Println(file.Name() + " > ")

		pixels, err := getPixels(frameDir + "/" + file.Name())
		if err != nil {
			fmt.Println("Error: Image could not be decoded")
			os.Exit(1)
		}

		var avg, median RGB
		var mode RGBList

		if genAvg {
			avg = getAverage(pixels)
			fmt.Printf("  Average: %s\n", avg.Hex())
		}

		if genMed {
			median = getMedian(pixels)
			fmt.Printf("  Median: %s\n", median.Hex())
		}

		if genMode > 0 {
			mode = getMode(pixels)
			fmt.Printf("  Mode: ")
			n := 5
			if len(mode) < n {
				n = len(mode)
			}
			for _, m := range mode[:n] {
				fmt.Printf(m.Hex() + ", ")
			}

			fmt.Println("...")
		}

		subPath := "frames/" + file.Name()

		result = append(result, Frame{subPath, avg, median, mode})
	}

	return result
}

func genLineImage(frames RGBList, filename string) {
	fmt.Println("Generating " + filename)
	img := image.NewRGBA(image.Rect(0, 0, width, len(frames)*lineHeight))
	for y, row := range frames {
		c := row.RGBA()
		for i := 0; i < lineHeight; i++ {
			for x := 0; x < width; x++ {
				img.Set(x, (y*lineHeight)+i, c)
			}
		}
	}

	outFile, err := os.Create(filename)
	if err != nil {
		fmt.Println("Error creating " + filename)
		fmt.Println(err)
		os.Exit(1)
	}

	png.Encode(outFile, img)

	outFile.Close()
}

func genLineColImage(frames []RGBList, filename string) {
	fmt.Println("Generating " + filename)
	img := image.NewRGBA(image.Rect(0, 0, width, len(frames)*lineHeight))
	// cols := len(frames[0])
	var colWidth int
	for y, row := range frames {
		colWidth = width / len(row)
		for i := 0; i < lineHeight; i++ {
			for j, col := range row {
				c := col.RGBA()
				for x := j * colWidth; x < (j*colWidth + colWidth); x++ {
					img.Set(x, (y*lineHeight)+i, c)
				}
			}
		}
	}

	outFile, err := os.Create(filename)
	if err != nil {
		fmt.Println("Error creating " + filename)
		fmt.Println(err)
		os.Exit(1)
	}

	png.Encode(outFile, img)

	outFile.Close()
}

func getVideoDuration(filename string) int {
	cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", filename)

	out, err := cmd.CombinedOutput()
	checkErr(err, "Failed to check video duration")

	d, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 32)
	checkErr(err)
	return int(d)
}

func checkErr(e error, msg ...string) {
	if e != nil {
		if len(msg) > 0 {
			fmt.Println(msg[0])
		}
		fmt.Println(e)
		os.Exit(1)
	}
}

var inputFile string
var outDir string
var width int
var height int
var lineHeight int
var framerate string
var thumbHeight int
var genAll bool
var genAvg bool
var avgSqr bool
var genMed bool
var genMode int

func main() {
	flag.StringVar(&inputFile, "i", "", "REQUIRED: Input video to be processed")
	flag.StringVar(&outDir, "o", "", "REQUIRED: Output directory to write results to")
	flag.IntVar(&width, "w", 720, "Width of output image.")
	flag.IntVar(&height, "h", 1280, "Desired height of output image. Will attempt to get as close as possible")
	flag.IntVar(&lineHeight, "lh", 1, "Height of each line in output image.")
	flag.IntVar(&thumbHeight, "th", 480, "Height to scale thumbnails to. Aspect ratio maintained")
	flag.BoolVar(&genAll, "all", false, "Generate all image options")
	flag.BoolVar(&genAvg, "avg", false, "Generate average image (default)")
	flag.BoolVar(&avgSqr, "avg-square", false, "Generate average image using squares algorithm")
	flag.BoolVar(&genMed, "median", false, "Generate median image")
	flag.IntVar(&genMode, "mode", 0, "Generate mode image with top N values")

	flag.Parse()

	inputFile = strings.ReplaceAll(inputFile, "\\", "/")
	outDir = strings.ReplaceAll(outDir, "\\", "/")

	if inputFile == "" {
		fmt.Print("-i [INPUT_FILE] is required!\n")
		os.Exit(1)
	}

	if outDir == "" {
		fmt.Print("-o [OUT_DIR] is required!\n")
		os.Exit(1)
	}

	if genAll {
		genAvg = true
		genMed = true
		if genMode == 0 {
			genMode = 1
		}
	}

	if !genAvg && !genMed && genMode == 0 {
		genAvg = true
	}

	res, err := isDir(outDir)
	if !res {
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		errDir := os.MkdirAll(outDir, 0755)
		if errDir != nil {
			fmt.Println(errDir)
			os.Exit(1)
		}
	}

	if err != nil {
		log.Fatal(err)
	}

	frameDir := fmt.Sprintf("%s/frames/", outDir)
	jsonFile := fmt.Sprintf("%s/data.json", outDir)

	duration := getVideoDuration(inputFile)

	framerate = fmt.Sprintf("%d/%d", (height / lineHeight), duration)
	fmt.Println(framerate)

	fmt.Println(framerate)

	createThumbs(inputFile, frameDir)

	image.RegisterFormat("png", "png", png.Decode, png.DecodeConfig)
	image.RegisterFormat("jpg", "jpg", jpeg.Decode, jpeg.DecodeConfig)

	frames := processFrames(frameDir)

	if genAvg {
		vals := make(RGBList, len(frames))
		for i, f := range frames {
			vals[i] = f.Average
		}
		genLineImage(vals, outDir+"/avg.png")
	}

	if genMed {
		vals := make(RGBList, len(frames))
		for i, f := range frames {
			vals[i] = f.Median
		}
		genLineImage(vals, outDir+"/med.png")
	}

	if genMode > 0 {
		vals := make([]RGBList, len(frames))
		for i, f := range frames {
			vals[i] = f.Mode
		}
		filename := fmt.Sprintf("/mode_%d.png", genMode)
		genLineColImage(vals, outDir+filename)
	}

	var jsonFrames []jsonFrame

	for _, frame := range frames {
		jsonFrames = append(jsonFrames, frame.toJSONFrame())
	}

	b, err := json.MarshalIndent(jsonFrames, "", "  ")

	if err != nil {
		fmt.Println("Error exporting to json!")
		os.Exit(1)
	}

	err = ioutil.WriteFile(jsonFile, b, 0644)
	checkErr(err)
}
