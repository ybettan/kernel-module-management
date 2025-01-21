package mic

import (
	"context"
	"errors"
	"reflect"

	"github.com/kubernetes-sigs/kernel-module-management/api/v1beta1"
	kmmv1beta1 "github.com/kubernetes-sigs/kernel-module-management/api/v1beta1"
	"github.com/kubernetes-sigs/kernel-module-management/internal/client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomock "go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("get", func() {

	const (
		micName      = "my-name"
		micNamespace = "my-namespace"
	)

	var (
		ctx        context.Context
		ctrl       *gomock.Controller
		mockClient *client.MockClient
		micAPIImpl *moduleImagesConfigAPI
	)

	BeforeEach(func() {
		ctx = context.Background()
		ctrl = gomock.NewController(GinkgoT())
		mockClient = client.NewMockClient(ctrl)
		micAPIImpl = &moduleImagesConfigAPI{
			client: mockClient,
		}
	})

	It("should fail if we fail to get the MIC", func() {

		mockClient.EXPECT().Get(ctx, types.NamespacedName{Name: micName, Namespace: micNamespace},
			gomock.Any()).Return(errors.New("some error"))

		mic, err := micAPIImpl.get(ctx, micName, micNamespace)

		Expect(err).To(MatchError(ContainSubstring("failed to get ModuleImagesConfig")))
		Expect(mic).To(BeNil())
	})

	It("should nil if the MIC doesn't exists", func() {

		mockClient.EXPECT().Get(ctx, types.NamespacedName{Name: micName, Namespace: micNamespace},
			gomock.Any()).Return(k8serrors.NewNotFound(schema.GroupResource{}, micName))

		mic, err := micAPIImpl.get(ctx, micName, micNamespace)

		Expect(err).NotTo(HaveOccurred())
		Expect(mic).To(BeNil())
	})

	It("should work as expected", func() {

		mockClient.EXPECT().Get(ctx, types.NamespacedName{Name: micName, Namespace: micNamespace}, gomock.Any()).DoAndReturn(
			func(_ interface{}, _ interface{}, mic *kmmv1beta1.ModuleImagesConfig, _ ...ctrlclient.GetOption) error {
				mic.ObjectMeta = metav1.ObjectMeta{Name: micName, Namespace: micNamespace}
				return nil
			},
		)

		mic, err := micAPIImpl.get(ctx, micName, micNamespace)

		Expect(err).NotTo(HaveOccurred())
		Expect(mic).NotTo(BeNil())
		Expect(mic.Name).To(Equal(micName))
		Expect(mic.Namespace).To(Equal(micNamespace))
	})
})

var _ = Describe("cerate", func() {

	const (
		micName      = "my-name"
		micNamespace = "my-namespace"
	)

	var (
		ctx        context.Context
		ctrl       *gomock.Controller
		mockClient *client.MockClient
		micAPIImpl *moduleImagesConfigAPI
	)

	BeforeEach(func() {
		ctx = context.Background()
		ctrl = gomock.NewController(GinkgoT())
		mockClient = client.NewMockClient(ctrl)
		micAPIImpl = &moduleImagesConfigAPI{
			client: mockClient,
		}
	})

	It("should fail if we fail to create the MIC object", func() {

		mockClient.EXPECT().Create(ctx, gomock.Any(), gomock.Any()).Return(errors.New("some error"))

		err := micAPIImpl.create(ctx, "", "", []kmmv1beta1.ModuleImageSpec{}, nil)

		Expect(err).To(MatchError(ContainSubstring("failed to create ModuleImagesConfig")))
	})

	It("should work as expected", func() {

		images := []v1beta1.ModuleImageSpec{
			{
				Image: "my-image:v1",
				Build: &kmmv1beta1.Build{},
				Sign:  &kmmv1beta1.Sign{},
			},
			{
				Image: "my-image@sha256:999",
				Build: &kmmv1beta1.Build{},
				Sign:  &kmmv1beta1.Sign{},
			},
		}

		imageRepoSecret := &v1.LocalObjectReference{}

		expectedMIC := &kmmv1beta1.ModuleImagesConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      micName,
				Namespace: micNamespace,
			},
			Spec: kmmv1beta1.ModuleImagesConfigSpec{
				Images:          images,
				ImageRepoSecret: imageRepoSecret,
			},
		}

		mockClient.EXPECT().Create(ctx, expectedMIC).Return(nil)

		err := micAPIImpl.create(ctx, micName, micNamespace, images, imageRepoSecret)

		Expect(err).NotTo(HaveOccurred())
	})

})

var _ = Describe("sliceToMap", func() {

	It("should work as expected", func() {

		images := []v1beta1.ModuleImageSpec{
			{
				Image: "my-image:v1",
				Build: &kmmv1beta1.Build{},
				Sign:  &kmmv1beta1.Sign{},
			},
			{
				Image: "my-image@sha256:999",
				Build: &kmmv1beta1.Build{},
				Sign:  &kmmv1beta1.Sign{},
			},
		}

		micAPIImpl := &moduleImagesConfigAPI{client: nil}

		m := micAPIImpl.sliceToMap(images)

		Expect(len(m)).To(Equal(len(images)))
		for _, img := range images {
			data, ok := m[img.Image]
			Expect(ok).To(BeTrue())
			Expect(data.generation).To(Equal(img.Generation))
			Expect(reflect.DeepEqual(data.build, img.Build)).To(BeTrue())
			Expect(reflect.DeepEqual(data.sign, img.Sign)).To(BeTrue())
		}
	})
})

var _ = Describe("HandleModuleImagesConfig", func() {

	It("should fail if we failed to get the MIC", func() {})
	It("should fail if we failed to create the MIC", func() {})
	It("should fail if we failed to patch the MIC", func() {})

	DescribeTable("should work as expected", func(labels map[string]string, key string) {
		//test here
	},
	//Entry("nil labels", nil, key),
	//Entry("empty labels", make(map[string]string), key),
	//Entry("existing label", map[string]string{key: "some-other-value"}, key),
	)
})
