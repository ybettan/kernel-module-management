package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	kmmv1beta1 "github.com/kubernetes-sigs/kernel-module-management/api/v1beta1"
	"github.com/kubernetes-sigs/kernel-module-management/internal/utils"
	"go.opentelemetry.io/otel/sdk/trace"
	cri "k8s.io/cri-client/pkg"
	//FIXME: use a released version rather than a commit sha
)

type remoteImageMounter struct {
	ociImageHelper ociImageMounterHelperAPI
	baseDir        string
	keyChain       authn.Keychain
	logger         logr.Logger
}

func NewRemoteImageMounter(baseDir string, keyChain authn.Keychain, logger logr.Logger) (ImageMounter, error) {

	//FIXME: get the endpoint from
	// ```
	// oc get node/minikube -o yaml | yq '.metadata.annotations["kubeadm.alpha.kubernetes.io/cri-socket"]'
	// ```
	runtimeEndpoint := "unix:///var/run/crio/crio.sock"
	imageService, err := cri.NewRemoteImageService(runtimeEndpoint, 1*time.Minute, trace.NewTracerProvider(), &logger)
	if err != nil {
		return nil, fmt.Errorf("could not connect to the image-service at %s: %v", runtimeEndpoint, err)
	}
	logger.Info("Successfully connected to the image-service", "endpoint", runtimeEndpoint)

	ociImageHelper := newOCIImageMounterHelper(logger, imageService)
	remoteImgMnt := &remoteImageMounter{
		ociImageHelper: ociImageHelper,
		baseDir:        baseDir,
		keyChain:       keyChain,
		logger:         logger,
	}

	return remoteImgMnt, nil
}

func (rim *remoteImageMounter) MountImage(ctx context.Context, imageName string, cfg *kmmv1beta1.ModuleConfig) (string, error) {
	logger := rim.logger.V(1).WithValues("image name", imageName)

	opts := []crane.Option{
		crane.WithContext(ctx),
		crane.WithAuthFromKeychain(rim.keyChain),
	}

	if cfg.InsecurePull {
		logger.Info(utils.WarnString("Pulling without TLS"))
		opts = append(opts, crane.Insecure)
	}

	logger.V(1).Info("Getting digest")

	remoteDigest, err := crane.Digest(imageName, opts...)
	if err != nil {
		return "", fmt.Errorf("could not get the digest for %s: %v", imageName, err)
	}

	dstDir := filepath.Join(rim.baseDir, imageName)
	digestPath := filepath.Join(dstDir, "digest")

	dstDirFS := filepath.Join(dstDir, "fs")
	cleanup := false

	logger.Info("Reading digest file", "path", digestPath)

	b, err := os.ReadFile(digestPath)
	if err != nil {
		if os.IsNotExist(err) {
			cleanup = true
		} else {
			return "", fmt.Errorf("could not open the digest file %s: %v", digestPath, err)
		}
	} else {
		logger.V(1).Info(
			"Comparing digests",
			"local file",
			string(b),
			"remote image",
			remoteDigest,
		)

		if string(b) == remoteDigest {
			logger.Info("Local file and remote digest are identical; skipping pull")
			return dstDirFS, nil
		} else {
			logger.Info("Local file and remote digest differ; pulling image")
			cleanup = true
		}
	}

	if cleanup {
		logger.Info("Cleaning up image directory", "path", dstDir)

		if err = os.RemoveAll(dstDir); err != nil {
			return "", fmt.Errorf("could not cleanup %s: %v", dstDir, err)
		}
	}

	if err = os.MkdirAll(dstDirFS, os.ModeDir|0755); err != nil {
		return "", fmt.Errorf("could not create the filesystem directory %s: %v", dstDirFS, err)
	}

	logger.V(1).Info("Pulling image")

	img, err := rim.ociImageHelper.pullOCIImage(ctx, logger, imageName)
	if err != nil {
		return "", fmt.Errorf("could not pull %s: %v", imageName, err)
	}

	err = rim.ociImageHelper.mountOCIImage(img, dstDirFS)
	if err != nil {
		return "", fmt.Errorf("failed mounting oci image: %v", err)
	}

	if err = ctx.Err(); err != nil {
		return "", fmt.Errorf("not writing digest file: %v", err)
	}

	logger.V(1).Info("Image written to the filesystem")

	digest, err := img.Digest()
	if err != nil {
		return "", fmt.Errorf("could not get the digest of the pulled image: %v", err)
	}

	digestStr := digest.String()

	logger.V(1).Info("Writing digest", "digest", digestStr)

	if err = os.WriteFile(digestPath, []byte(digestStr), 0644); err != nil {
		return "", fmt.Errorf("could not write the digest file at %s: %v", digestPath, err)
	}

	return dstDirFS, nil
}
