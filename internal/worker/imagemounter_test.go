package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/cri-client/pkg/fake"
)

func sameFiles(a, b string) (bool, error) {
	fiA, err := os.Stat(a)
	if err != nil {
		return false, fmt.Errorf("could not stat() the first file: %v", err)
	}

	fiB, err := os.Stat(b)
	if err != nil {
		return false, fmt.Errorf("could not stat() the second file: %v", err)
	}

	return os.SameFile(fiA, fiB), nil
}

var _ = Describe("ImageMounter_mountOCIImage", func() {

	It("good flow", func() {
		tmpDir := GinkgoT().TempDir()

		ociImage, err := crane.Append(empty.Image, "testdata/archive.tar")
		Expect(err).NotTo(HaveOccurred())

		oimh := newOCIImageMounterHelper(GinkgoLogr, nil)
		err = oimh.mountOCIImage(ociImage, tmpDir)

		Expect(err).NotTo(HaveOccurred())
		Expect(filepath.Join(tmpDir, "subdir")).To(BeADirectory())
		Expect(filepath.Join(tmpDir, "subdir", "subsubdir")).To(BeADirectory())

		Expect(filepath.Join(tmpDir, "a")).To(BeARegularFile())
		Expect(filepath.Join(tmpDir, "subdir", "b")).To(BeARegularFile())
		Expect(filepath.Join(tmpDir, "subdir", "subsubdir", "c")).To(BeARegularFile())

		Expect(
			os.Readlink(filepath.Join(tmpDir, "lib-modules-symlink")),
		).To(
			Equal("/lib/modules"),
		)

		Expect(
			os.Readlink(filepath.Join(tmpDir, "symlink")),
		).To(
			Equal("a"),
		)

		Expect(
			sameFiles(filepath.Join(tmpDir, "link"), filepath.Join(tmpDir, "a")),
		).To(
			BeTrue(),
		)
	})
})

var _ = FDescribe("ImageMounter_pullOCIImage", func() {

	It("should fail if it fails to pull the image", func() {

		fakeImageService := fake.NewFakeRemoteRuntime().ImageService

		fakeImages := []string{"quay.io/org/kmm-kmod:tag1", "quay.io/org/kmm-kmod:tag2"}
		fakeImageService.SetFakeImages(fakeImages)

		//imgs, err := fakeImageService.ListImages(ctx, nil)
		//if err != nil {
		//	return nil, fmt.Errorf("could not list images: %v", err)
		//}

		oimh := newOCIImageMounterHelper(GinkgoLogr, fakeImageService)
		_, err := oimh.pullOCIImage(context.Background(), GinkgoLogr, "quay.io/org/kmm-kmod:no-such-tag")
		Expect(err).To(HaveOccurred())
	})

	It("should work as expected", func() {
	})
})
