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
	"strings"
)

//Frame is base object to hold frame info
// type Frame struct {
// 	Path    string
// 	Average color.RGBA
// 	Median  color.RGBA
// }

//Frame is base object to hold frame info
type Frame struct {
	Path    string
	Average color.RGBA
	Median  color.RGBA
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
func getPixels(filename string) ([][]color.RGBA, error) {
	img := openImage(filename)

	bounds := img.Bounds()
	width, height := bounds.Max.X, bounds.Max.Y

	var pixels [][]color.RGBA
	for y := 0; y < height; y++ {
		var row []color.RGBA
		for x := 0; x < width; x++ {
			row = append(row, rgbaToPixel(img.At(x, y).RGBA()))
		}
		pixels = append(pixels, row)
	}

	return pixels, nil
}

// img.At(x, y).RGBA() returns four uint32 values; we want a Pixel
func rgbaToPixel(r uint32, g uint32, b uint32, a uint32) color.RGBA {
	return color.RGBA{uint8(r / 257), uint8(g / 257), uint8(b / 257), 255}
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
	opts := []string{"-progress", "pipe:1", "-i", input, "-vf", "fps=1/10,scale=-2:480", outFormat}
	ffmpeg(opts...)
}

func genAverage(pixels [][]color.RGBA) color.RGBA {
	var r, g, b float64

	h := len(pixels)
	w := len(pixels[0])
	total := h * w

	result := color.RGBA{0, 0, 0, 255}

	if avgSqr { //squared algorithm
		// https://sighack.com/post/averaging-rgb-colors-the-right-way?fbclid=IwAR3T1vH62sG1U1JuoSgOJ5-7XqtqekHKmp_Ebw6JwXczteQVkOdgpW5T4Sw
		for _, row := range pixels {
			for _, p := range row {
				r += math.Pow(float64(p.R), 2)
				g += math.Pow(float64(p.G), 2)
				b += math.Pow(float64(p.B), 2)
			}
		}

		result.R = uint8(math.Sqrt(r / float64(total)))
		result.G = uint8(math.Sqrt(g / float64(total)))
		result.B = uint8(math.Sqrt(b / float64(total)))
	} else {
		for _, row := range pixels {
			for _, p := range row {
				r += float64(p.R)
				g += float64(p.G)
				b += float64(p.B)
			}
		}

		result.R = uint8(math.Floor(r / float64(total)))
		result.G = uint8(math.Floor(g / float64(total)))
		result.B = uint8(math.Floor(b / float64(total)))
	}

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
		fmt.Print(file.Name() + " > ")

		pixels, err := getPixels(frameDir + "/" + file.Name())
		if err != nil {
			fmt.Println("Error: Image could not be decoded")
			os.Exit(1)
		}

		var avg, median color.RGBA

		if genAvg {
			avg = genAverage(pixels)
		}

		if genMedian {

		}

		subPath := "frames/" + file.Name()

		result = append(result, Frame{subPath, avg, median})
		fmt.Printf("rgb(%d, %d, %d) | #%02X%02X%02X\n", avg.R, avg.G, avg.B, avg.R, avg.G, avg.B)
	}

	return result
}

func genAvgLineImage(frames []Frame, filename string) {
	fmt.Println("Generating line image...")
	w := 720
	lineHeight := 1
	img := image.NewRGBA(image.Rect(0, 0, w, len(frames)*lineHeight))
	for y, row := range frames {
		for i := 0; i < lineHeight; i++ {
			for x := 0; x < w; x++ {
				img.Set(x, (y*lineHeight)+i, row.Average)
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
var genAll bool
var genAvg bool
var avgSqr bool
var genMedian bool

func main() {
	flag.StringVar(&inputFile, "i", "", "REQUIRED: Input video to be processed")
	flag.StringVar(&outDir, "o", "", "REQUIRED: Output directory to write results to")
	flag.IntVar(&width, "w", 720, "Width of output image.")
	flag.IntVar(&width, "h", 1280, "Height of output image.")
	flag.BoolVar(&genAll, "all", false, "Generate all image options")
	flag.BoolVar(&genAvg, "avg", true, "Generate average image")
	flag.BoolVar(&avgSqr, "avg-square", false, "Generate average image using squares algorithm")
	flag.BoolVar(&genMedian, "median", false, "Generate median image")

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
		genMedian = true
	}

	res, err := isDir(outDir)
	if !res {
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Printf("Directory %s does not exist!\n", outDir)
		os.Exit(1)
	}

	frameDir := fmt.Sprintf("%s/frames/", outDir)
	jsonFile := fmt.Sprintf("%s/data.json", outDir)

	createThumbs(inputFile, frameDir)

	image.RegisterFormat("png", "png", png.Decode, png.DecodeConfig)
	image.RegisterFormat("jpg", "jpg", jpeg.Decode, jpeg.DecodeConfig)

	frames := processFrames(frameDir)

	// genAvgLineImage(frames, outDir+"/lines.png")

	b, err := json.MarshalIndent(frames, "", "  ")

	if err != nil {
		fmt.Println("Error exporting to json!")
		os.Exit(1)
	}

	err = ioutil.WriteFile(jsonFile, b, 0644)
	checkErr(err)
}
