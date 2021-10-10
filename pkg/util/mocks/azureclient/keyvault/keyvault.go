// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/Azure/ARO-RP/pkg/util/azureclient/keyvault (interfaces: BaseClient)

// Package mock_keyvault is a generated GoMock package.
package mock_keyvault

import (
	context "context"
	reflect "reflect"

	keyvault "github.com/Azure/azure-sdk-for-go/services/keyvault/v7.0/keyvault"
	gomock "github.com/golang/mock/gomock"
)

// MockBaseClient is a mock of BaseClient interface.
type MockBaseClient struct {
	ctrl     *gomock.Controller
	recorder *MockBaseClientMockRecorder
}

// MockBaseClientMockRecorder is the mock recorder for MockBaseClient.
type MockBaseClientMockRecorder struct {
	mock *MockBaseClient
}

// NewMockBaseClient creates a new mock instance.
func NewMockBaseClient(ctrl *gomock.Controller) *MockBaseClient {
	mock := &MockBaseClient{ctrl: ctrl}
	mock.recorder = &MockBaseClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockBaseClient) EXPECT() *MockBaseClientMockRecorder {
	return m.recorder
}

// CreateCertificate mocks base method.
func (m *MockBaseClient) CreateCertificate(arg0 context.Context, arg1, arg2 string, arg3 keyvault.CertificateCreateParameters) (keyvault.CertificateOperation, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateCertificate", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(keyvault.CertificateOperation)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateCertificate indicates an expected call of CreateCertificate.
func (mr *MockBaseClientMockRecorder) CreateCertificate(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateCertificate", reflect.TypeOf((*MockBaseClient)(nil).CreateCertificate), arg0, arg1, arg2, arg3)
}

// DeleteCertificate mocks base method.
func (m *MockBaseClient) DeleteCertificate(arg0 context.Context, arg1, arg2 string) (keyvault.DeletedCertificateBundle, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteCertificate", arg0, arg1, arg2)
	ret0, _ := ret[0].(keyvault.DeletedCertificateBundle)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DeleteCertificate indicates an expected call of DeleteCertificate.
func (mr *MockBaseClientMockRecorder) DeleteCertificate(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteCertificate", reflect.TypeOf((*MockBaseClient)(nil).DeleteCertificate), arg0, arg1, arg2)
}

// GetCertificateOperation mocks base method.
func (m *MockBaseClient) GetCertificateOperation(arg0 context.Context, arg1, arg2 string) (keyvault.CertificateOperation, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetCertificateOperation", arg0, arg1, arg2)
	ret0, _ := ret[0].(keyvault.CertificateOperation)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetCertificateOperation indicates an expected call of GetCertificateOperation.
func (mr *MockBaseClientMockRecorder) GetCertificateOperation(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetCertificateOperation", reflect.TypeOf((*MockBaseClient)(nil).GetCertificateOperation), arg0, arg1, arg2)
}

// GetCertificates mocks base method.
func (m *MockBaseClient) GetCertificates(arg0 context.Context, arg1 string, arg2 *int32, arg3 *bool) (keyvault.CertificateListResultPage, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetCertificates", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(keyvault.CertificateListResultPage)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetCertificates indicates an expected call of GetCertificates.
func (mr *MockBaseClientMockRecorder) GetCertificates(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetCertificates", reflect.TypeOf((*MockBaseClient)(nil).GetCertificates), arg0, arg1, arg2, arg3)
}

// GetSecret mocks base method.
func (m *MockBaseClient) GetSecret(arg0 context.Context, arg1, arg2, arg3 string) (keyvault.SecretBundle, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetSecret", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(keyvault.SecretBundle)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetSecret indicates an expected call of GetSecret.
func (mr *MockBaseClientMockRecorder) GetSecret(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetSecret", reflect.TypeOf((*MockBaseClient)(nil).GetSecret), arg0, arg1, arg2, arg3)
}

// GetSecretVersions mocks base method.
func (m *MockBaseClient) GetSecretVersions(arg0 context.Context, arg1, arg2 string, arg3 *int32) ([]keyvault.SecretItem, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetSecretVersions", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].([]keyvault.SecretItem)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetSecretVersions indicates an expected call of GetSecretVersions.
func (mr *MockBaseClientMockRecorder) GetSecretVersions(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetSecretVersions", reflect.TypeOf((*MockBaseClient)(nil).GetSecretVersions), arg0, arg1, arg2, arg3)
}

// GetSecrets mocks base method.
func (m *MockBaseClient) GetSecrets(arg0 context.Context, arg1 string, arg2 *int32) ([]keyvault.SecretItem, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetSecrets", arg0, arg1, arg2)
	ret0, _ := ret[0].([]keyvault.SecretItem)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetSecrets indicates an expected call of GetSecrets.
func (mr *MockBaseClientMockRecorder) GetSecrets(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetSecrets", reflect.TypeOf((*MockBaseClient)(nil).GetSecrets), arg0, arg1, arg2)
}

// SetCertificateIssuer mocks base method.
func (m *MockBaseClient) SetCertificateIssuer(arg0 context.Context, arg1, arg2 string, arg3 keyvault.CertificateIssuerSetParameters) (keyvault.IssuerBundle, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetCertificateIssuer", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(keyvault.IssuerBundle)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// SetCertificateIssuer indicates an expected call of SetCertificateIssuer.
func (mr *MockBaseClientMockRecorder) SetCertificateIssuer(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetCertificateIssuer", reflect.TypeOf((*MockBaseClient)(nil).SetCertificateIssuer), arg0, arg1, arg2, arg3)
}

// SetSecret mocks base method.
func (m *MockBaseClient) SetSecret(arg0 context.Context, arg1, arg2 string, arg3 keyvault.SecretSetParameters) (keyvault.SecretBundle, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetSecret", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(keyvault.SecretBundle)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// SetSecret indicates an expected call of SetSecret.
func (mr *MockBaseClientMockRecorder) SetSecret(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetSecret", reflect.TypeOf((*MockBaseClient)(nil).SetSecret), arg0, arg1, arg2, arg3)
}
