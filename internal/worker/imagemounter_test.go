package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/umoci"
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

var _ = Describe("ImageMounter_pullOCIImage", func() {

	It("should fail if it fails to pull the image", func() {

		fakeImageService := fake.NewFakeRemoteRuntime().ImageService
		fakeImageService.InjectError("PullImage", errors.New("random error"))

		oimh := newOCIImageMounterHelper(GinkgoLogr, fakeImageService)
		_, err := oimh.pullOCIImage(context.Background(), GinkgoLogr, "quay.io/org/kmm-kmod:no-such-tag")
		Expect(err).To(HaveOccurred())
	})

	//FIt("should work as expected", func() {

	//	manifestFile, err := os.Open("/home/ybettan/go/src/github.com/storage/overlay-images/6c8ef9aa7b75b03af093f5e48c6c7c319f92736f319b7d210f5d9f7b6965159a/manifest")
	//	Expect(err).NotTo(HaveOccurred())

	//	manifest, err := v1.ParseManifest(manifestFile)
	//	Expect(err).NotTo(HaveOccurred())

	//})

	FIt("umoci", func() {

		engine, err := umoci.OpenLayout("image-spec")
		Expect(err).NotTo(HaveOccurred())

		digests, err := engine.ListBlobs(context.Background())
		Expect(err).NotTo(HaveOccurred())

		fmt.Println(digests)
	})

})
