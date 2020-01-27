package pnm

import (
	"image"
	_ "image/png"
	"os"
	"testing"
)

type tbParam interface {
	Fatal(args ...interface{})
}

func TestDecodeRawRGB(t *testing.T) {
	pnmFile := openFile(t, "testdata/test_rgb_raw.ppm")
	pngFile := openFile(t, "testdata/test_rgb.png")
	defer pngFile.Close()
	defer pnmFile.Close()
	pnmImage, format, err := image.Decode(pnmFile)
	pngImage, _, err := image.Decode(pngFile)
	pnm := pnmImage.(*image.RGBA)
	png := pngImage.(*image.RGBA)

	if err != nil {
		t.Fatal(err)
	}

	if format != "ppm raw (rgb)" {
		t.Fatal("Unexpected format:", format, "expecting ppm raw (rgb)")
	}

	if len(png.Pix) != len(pnm.Pix) {
		t.Fatal("Wrong pixel count:", len(pnm.Pix), "expected: ", len(png.Pix))
	}

	for i := 0; i < len(png.Pix); i++ {
		//t.Log("(", png.Pix[i], ",", pnm.Pix[i], ")")
		if png.Pix[i] != pnm.Pix[i] {
			t.Fatal("Incorrect pixel at position", i, "found", pnm.Pix[i], "but expected", png.Pix[i])
		}
	}

}

func BenchmarkDecodePlainBW(b *testing.B) {
	benchmarkPnm(b, "testdata/test_bw_plain.pbm")
}

func BenchmarkDecodeRawBW(b *testing.B) {
	benchmarkPnm(b, "testdata/test_bw_raw.pbm")
}

func BenchmarkDecodePlainGrayscale(b *testing.B) {
	benchmarkPnm(b, "testdata/test_grayscale_plain.pgm")
}

func BenchmarkDecodeRawGrayscale(b *testing.B) {
	benchmarkPnm(b, "testdata/test_grayscale_raw.pgm")
}

func BenchmarkDecodePlainRGB(b *testing.B) {
	benchmarkPnm(b, "testdata/test_rgb_plain.ppm")
}

func BenchmarkDecodeRawRGB(b *testing.B) {
	benchmarkPnm(b, "testdata/test_rgb_raw.ppm")
}

func benchmarkPnm(b *testing.B, fileName string) {
	b.StopTimer()

	for i := 0; i < b.N; i++ {

		file := openFile(b, fileName)
		b.StartTimer()

		image.Decode(file)

		b.StopTimer()
		file.Close()
	}

}

func openFile(tb tbParam, fileName string) *os.File {
	file, err := os.Open(fileName)

	if err != nil {
		tb.Fatal(err)
	}

	return file
}
