package mic

//import (
//	"context"
//
//	"github.com/kubernetes-sigs/kernel-module-management/internal/client"
//	. "github.com/onsi/ginkgo/v2"
//	. "github.com/onsi/gomega"
//	gomock "go.uber.org/mock/gomock"
//)
//
//var _ = Describe("HandleModuleImagesConfig", func() {
//
//	const (
//		micName      = "my-name"
//		micNamespace = "my-namespace"
//	)
//
//	var (
//		ctx        context.Context
//		ctrl       *gomock.Controller
//		mockClient *client.MockClient
//		micAPIImpl *moduleImagesConfigAPI
//	)
//
//	BeforeEach(func() {
//		ctx = context.Background()
//		ctrl = gomock.NewController(GinkgoT())
//		mockClient = client.NewMockClient(ctrl)
//		micAPIImpl = &moduleImagesConfigAPI{
//			client: mockClient,
//		}
//	})
//
//	It("should fail if we failed to get the MIC", func() {})
//	It("should fail if we failed to create the MIC", func() {})
//	It("should fail if we failed to patch the MIC", func() {})
//
//	DescribeTable("should work as expected", func(labels map[string]string, key string) {
//		//test here
//	},
//	//Entry("nil labels", nil, key),
//	//Entry("empty labels", make(map[string]string), key),
//	//Entry("existing label", map[string]string{key: "some-other-value"}, key),
//	)
//})
