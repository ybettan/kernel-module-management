package mic

import (
	"context"
	"fmt"
	"reflect"

	"github.com/kubernetes-sigs/kernel-module-management/api/v1beta1"
	kmmv1beta1 "github.com/kubernetes-sigs/kernel-module-management/api/v1beta1"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -source=mic.go -package=mic -destination=mock_mic.go

type ModuleImagesConfigAPI interface {
	HandleModuleImagesConfig(ctx context.Context, name, ns string, images []kmmv1beta1.ModuleImageSpec,
		imageRepoSecret *v1.LocalObjectReference) error
}

type moduleImagesConfigAPI struct {
	client client.Client
}

func NewModuleImagesConfigAPI(client client.Client) ModuleImagesConfigAPI {
	return &moduleImagesConfigAPI{
		client: client,
	}
}

func (mica *moduleImagesConfigAPI) get(ctx context.Context, name, ns string) (*kmmv1beta1.ModuleImagesConfig, error) {
	mic := kmmv1beta1.ModuleImagesConfig{}
	err := mica.client.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, &mic)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get ModuleImagesConfig %s/%s: %v", ns, name, err)
		}
		return nil, nil
	}
	return &mic, nil
}

func (mica *moduleImagesConfigAPI) create(ctx context.Context, name, ns string, images []kmmv1beta1.ModuleImageSpec, imageRepoSecret *v1.LocalObjectReference) error {

	mic := kmmv1beta1.ModuleImagesConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: kmmv1beta1.ModuleImagesConfigSpec{
			Images:          images,
			ImageRepoSecret: imageRepoSecret,
		},
	}
	err := mica.client.Create(ctx, &mic)
	if err != nil {
		return fmt.Errorf("failed to create ModuleImagesConfig %s/%s: %v", ns, name, err)
	}
	return nil
}

type moduleImageSpecData struct {
	generation int
	build      *kmmv1beta1.Build
	sign       *kmmv1beta1.Sign
}

func (mica *moduleImagesConfigAPI) sliceToMap(images []kmmv1beta1.ModuleImageSpec) map[string]moduleImageSpecData {

	res := map[string]moduleImageSpecData{}
	for _, img := range images {
		misd := moduleImageSpecData{
			generation: img.Generation,
			build:      img.Build,
			sign:       img.Sign,
		}
		res[img.Image] = misd
	}

	return res
}

// If the MIC doesn't exist - create it
// If there is a new image - add it to the spec with generation=1
// If there is a new version of an image - modify it with generation++
// Remove unused images
func (mica *moduleImagesConfigAPI) HandleModuleImagesConfig(ctx context.Context, name, ns string,
	images []kmmv1beta1.ModuleImageSpec, imageRepoSecret *v1.LocalObjectReference) error {

	moduleImagesConfig, err := mica.get(ctx, name, ns)
	if err != nil {
		return fmt.Errorf("failed to get moduleImagesConfig %s/%s: %v", ns, name, err)
	}

	// MIC doesn't exist - create it
	if moduleImagesConfig == nil {
		if err := mica.create(ctx, name, ns, images, imageRepoSecret); err != nil {
			return fmt.Errorf("failed to create moduleImagesConfig %s/%s : %v", ns, name, err)
		}
		return nil
	}

	// Update an existing MIC
	var updatedImages []v1beta1.ModuleImageSpec
	currentImages := mica.sliceToMap(moduleImagesConfig.Spec.Images)
	for _, img := range images {
		if _, ok := currentImages[img.Image]; !ok {
			// image is not in sepc - add it
			img.Generation = 1
			updatedImages = append(updatedImages, img)
		} else {
			// image is in spec - if needed, update the spec and bump the generation field
			currentImg := v1beta1.ModuleImageSpec{
				Image: img.Image,
				Build: currentImages[img.Image].build,
				Sign:  currentImages[img.Image].sign,
			}
			if !reflect.DeepEqual(img, currentImg) {
				img.Generation = currentImages[img.Image].generation + 1
				updatedImages = append(updatedImages, img)
			}
		}
	}

	newMIC := moduleImagesConfig.DeepCopy()
	newMIC.Spec.Images = updatedImages
	return mica.client.Patch(ctx, moduleImagesConfig, client.MergeFrom(newMIC))
}
