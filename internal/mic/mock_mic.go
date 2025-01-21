// Code generated by MockGen. DO NOT EDIT.
// Source: mic.go
//
// Generated by this command:
//
//	mockgen -source=mic.go -package=mic -destination=mock_mic.go
//
// Package mic is a generated GoMock package.
package mic

import (
	context "context"
	reflect "reflect"

	v1beta1 "github.com/kubernetes-sigs/kernel-module-management/api/v1beta1"
	gomock "go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
)

// MockModuleImagesConfigAPI is a mock of ModuleImagesConfigAPI interface.
type MockModuleImagesConfigAPI struct {
	ctrl     *gomock.Controller
	recorder *MockModuleImagesConfigAPIMockRecorder
}

// MockModuleImagesConfigAPIMockRecorder is the mock recorder for MockModuleImagesConfigAPI.
type MockModuleImagesConfigAPIMockRecorder struct {
	mock *MockModuleImagesConfigAPI
}

// NewMockModuleImagesConfigAPI creates a new mock instance.
func NewMockModuleImagesConfigAPI(ctrl *gomock.Controller) *MockModuleImagesConfigAPI {
	mock := &MockModuleImagesConfigAPI{ctrl: ctrl}
	mock.recorder = &MockModuleImagesConfigAPIMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockModuleImagesConfigAPI) EXPECT() *MockModuleImagesConfigAPIMockRecorder {
	return m.recorder
}

// HandleModuleImagesConfig mocks base method.
func (m *MockModuleImagesConfigAPI) HandleModuleImagesConfig(ctx context.Context, name, ns string, images []v1beta1.ModuleImageSpec, imageRepoSecret *v1.LocalObjectReference) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "HandleModuleImagesConfig", ctx, name, ns, images, imageRepoSecret)
	ret0, _ := ret[0].(error)
	return ret0
}

// HandleModuleImagesConfig indicates an expected call of HandleModuleImagesConfig.
func (mr *MockModuleImagesConfigAPIMockRecorder) HandleModuleImagesConfig(ctx, name, ns, images, imageRepoSecret any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "HandleModuleImagesConfig", reflect.TypeOf((*MockModuleImagesConfigAPI)(nil).HandleModuleImagesConfig), ctx, name, ns, images, imageRepoSecret)
}
