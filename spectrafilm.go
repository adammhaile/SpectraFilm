package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
)

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

func createThumbs(input string, output string) {
	frameDir := fmt.Sprintf("%s/frames", output)

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

	outFormat := frameDir + "/img%06d.jpg"
	opts := []string{"-progress", "pipe:1", "-i", input, "-vf", "fps=1/10,scale=-2:720", outFormat}
	ffmpeg(opts...)
}

func main() {
	var inputFile string
	var outDir string
	flag.StringVar(&inputFile, "i", "", "Input video to be processed")
	flag.StringVar(&outDir, "o", "", "Output directory to write results to")

	flag.Parse()

	inputFile = strings.ReplaceAll(inputFile, "\\", "/")
	outDir = strings.ReplaceAll(outDir, "\\", "/")

	fmt.Println("Input: ", inputFile)
	fmt.Println("Out Dir: ", outDir)

	res, err := isDir(outDir)
	if !res {
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Printf("Directory %s does not exist!\n", outDir)
		os.Exit(1)
	}

	createThumbs(inputFile, outDir)
}
