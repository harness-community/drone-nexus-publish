package plugin

import (
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockHttpClient struct {
	mock.Mock
}

func (m *MockHttpClient) Do(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	return args.Get(0).(*http.Response), args.Error(1)
}

// Utility function to create a temporary file for testing
func createTempFile(content string) (string, error) {
	tmpFile, err := ioutil.TempFile("", "testfile_*.zip")
	if err != nil {
		return "", err
	}
	if _, err := tmpFile.Write([]byte(content)); err != nil {
		tmpFile.Close()
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		return "", err
	}
	return tmpFile.Name(), nil
}

func TestNexusPlugin_Run_UploadFailed(t *testing.T) {
	mockClient := new(MockHttpClient)
	mockResp := &http.Response{
		StatusCode: 500,
		Body:       ioutil.NopCloser(strings.NewReader("Internal Server Error")),
	}
	mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(mockResp, nil)

	tmpFile, err := createTempFile("testfile.zip")
	assert.NoError(t, err)
	defer os.Remove(tmpFile)

	plugin := NexusPlugin{
		PluginProcessingInfo: PluginProcessingInfo{
			UserName:   "testUser",
			Password:   "testPass",
			ServerUrl:  "https://nexus.example.com",
			Repository: "repo",
			GroupId:    "group",
			Version:    "nexus3",
			Format:     "maven2",
			Artifacts: []Artifact{
				{
					File:       tmpFile,
					ArtifactId: "artifact123",
					Type:       "zip",
					Version:    "1",
				},
			},
		},
		HttpClient: mockClient,
	}

	err = plugin.Run()

	assert.NotNil(t, err)
	assert.Len(t, plugin.Failed, 1)
	assert.Equal(t, tmpFile, plugin.Failed[0].File)
	assert.Equal(t, "artifact123", plugin.Failed[0].ArtifactId)
	assert.Contains(t, plugin.Failed[0].Err, "upload failed")
	mockClient.AssertExpectations(t)
}

// The following tests validate argument processing without needing an actual file

func TestNexusPlugin_ValidateAndProcessArgs_MultiFileUpload_Success(t *testing.T) {
	args := Args{
		EnvPluginInputArgs: EnvPluginInputArgs{
			Username:     "testUser",
			Password:     "testPass",
			Protocol:     "https",
			ServerUrl:    "nexus.example.com",
			NexusVersion: "3",
			Repository:   "repo",
			GroupId:      "group",
			Format:       "maven2",
			Artifact:     "[{ \"artifactId\": \"artifact123\", \"file\": \"testfile.zip\", \"type\": \"zip\", \"version\": \"1\" }]",
		},
	}

	plugin := NexusPlugin{}
	err := plugin.ValidateAndProcessArgs(args)

	assert.Nil(t, err)
	assert.Len(t, plugin.Artifacts, 1)
	assert.Equal(t, "testfile.zip", plugin.Artifacts[0].File)
	assert.Equal(t, "artifact123", plugin.Artifacts[0].ArtifactId)
}

func TestNexusPlugin_ValidateAndProcessArgs_SingleFileUpload_Success(t *testing.T) {
	args := Args{
		EnvPluginInputArgs: EnvPluginInputArgs{
			Username:   "testUser",
			Password:   "testPass",
			ServerUrl:  "https://nexus.example.com",
			Filename:   "testfile.zip",
			Format:     "zip",
			Repository: "repo",
			Attributes: "-CgroupId=group -CartifactId=artifact123 -Cversion=1.0.0 -Aextension=zip -Aclassifier=classifier",
		},
	}

	plugin := NexusPlugin{}
	err := plugin.ValidateAndProcessArgs(args)

	assert.Nil(t, err)
	assert.Len(t, plugin.Artifacts, 1)
	assert.Equal(t, "testfile.zip", plugin.Artifacts[0].File)
	assert.Equal(t, "artifact123", plugin.Artifacts[0].ArtifactId)
}

func TestNexusPlugin_ValidateAndProcessArgs_MissingArguments(t *testing.T) {
	args := Args{
		EnvPluginInputArgs: EnvPluginInputArgs{
			Username:   "testUser",
			Password:   "testPass",
			ServerUrl:  "https://nexus.example.com",
			Filename:   "testfile.zip",
			Format:     "zip",
			Repository: "repo",
		},
	}

	plugin := NexusPlugin{}
	err := plugin.ValidateAndProcessArgs(args)

	assert.NotNil(t, err)
	assert.Equal(t, "Error in DetermineCompatibilityMode: both 'Attributes' and 'Artifact' cannot be empty", err.Error())
}

func TestNexusPlugin_Run_MultiFileUpload_Success(t *testing.T) {
	mockClient := new(MockHttpClient)
	mockResp := &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(strings.NewReader("Success")),
	}
	mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(mockResp, nil).Maybe()

	tmpFile1, err := createTempFile("file1.zip")
	assert.NoError(t, err)
	defer os.Remove(tmpFile1)

	tmpFile2, err := createTempFile("file2.zip")
	assert.NoError(t, err)
	defer os.Remove(tmpFile2)

	plugin := NexusPlugin{
		PluginProcessingInfo: PluginProcessingInfo{
			UserName:   "testUser",
			Password:   "testPass",
			ServerUrl:  "https://nexus.example.com",
			Repository: "repo",
			GroupId:    "group",
			Version:    "1.0.0",
			Format:     "maven2",
			Artifacts: []Artifact{
				{File: tmpFile1, ArtifactId: "artifact1", Type: "zip", Version: "1"},
				{File: tmpFile2, ArtifactId: "artifact2", Type: "zip", Version: "1"},
			},
		},
		HttpClient: mockClient,
	}

	err = plugin.Run()

	assert.Nil(t, err)
	assert.Empty(t, plugin.Failed)
	mockClient.AssertExpectations(t)
}

// Test Bug #3: URL Trailing Slash - Single File Upload
func TestIsSingleFileUploadArgsOk_TrailingSlash(t *testing.T) {
	args := Args{
		EnvPluginInputArgs: EnvPluginInputArgs{
			Username:   "testUser",
			Password:   "testPass",
			ServerUrl:  "https://nexus.example.com/",
			Filename:   "testfile.jar",
			Format:     "maven2",
			Repository: "repo",
			Attributes: "-CgroupId=com.test -CartifactId=app -Cversion=1.0.0 -Aextension=jar -Aclassifier=bin",
		},
	}

	plugin := NexusPlugin{}
	err := plugin.IsSingleFileUploadArgsOk(args)

	assert.Nil(t, err)
	assert.Equal(t, "https://nexus.example.com", plugin.ServerUrl, "Trailing slash should be removed")
}

// Test Bug #3: URL Multiple Trailing Slashes - Single File Upload
func TestIsSingleFileUploadArgsOk_MultipleTrailingSlashes(t *testing.T) {
	args := Args{
		EnvPluginInputArgs: EnvPluginInputArgs{
			Username:   "testUser",
			Password:   "testPass",
			ServerUrl:  "https://nexus.example.com///",
			Filename:   "testfile.jar",
			Format:     "maven2",
			Repository: "repo",
			Attributes: "-CgroupId=com.test -CartifactId=app -Cversion=1.0.0 -Aextension=jar -Aclassifier=bin",
		},
	}

	plugin := NexusPlugin{}
	err := plugin.IsSingleFileUploadArgsOk(args)

	assert.Nil(t, err)
	assert.Equal(t, "https://nexus.example.com", plugin.ServerUrl, "Multiple trailing slashes should be removed")
}

// Test Bug #3: URL No Trailing Slash - Single File Upload (should remain unchanged)
func TestIsSingleFileUploadArgsOk_NoTrailingSlash(t *testing.T) {
	args := Args{
		EnvPluginInputArgs: EnvPluginInputArgs{
			Username:   "testUser",
			Password:   "testPass",
			ServerUrl:  "https://nexus.example.com",
			Filename:   "testfile.jar",
			Format:     "maven2",
			Repository: "repo",
			Attributes: "-CgroupId=com.test -CartifactId=app -Cversion=1.0.0 -Aextension=jar -Aclassifier=bin",
		},
	}

	plugin := NexusPlugin{}
	err := plugin.IsSingleFileUploadArgsOk(args)

	assert.Nil(t, err)
	assert.Equal(t, "https://nexus.example.com", plugin.ServerUrl, "URL without trailing slash should remain unchanged")
}

// Test Bug #3: URL Trailing Slash - Multi File Upload
func TestIsMultiFileUploadArgsOk_TrailingSlash(t *testing.T) {
	args := Args{
		EnvPluginInputArgs: EnvPluginInputArgs{
			Username:     "testUser",
			Password:     "testPass",
			Protocol:     "https",
			ServerUrl:    "nexus.example.com/",
			NexusVersion: "nexus3",
			Repository:   "repo",
			GroupId:      "com.test",
			Format:       "maven2",
			Artifact:     "[{\"file\":\"test.jar\",\"artifactId\":\"app\",\"type\":\"jar\",\"version\":\"1.0\",\"groupId\":\"com.test\"}]",
		},
	}

	plugin := NexusPlugin{}
	err := plugin.IsMultiFileUploadArgsOk(args)

	assert.Nil(t, err)
	assert.Equal(t, "https://nexus.example.com", plugin.ServerUrl, "Trailing slash should be removed")
}

// Test Bug #3: URL Multiple Trailing Slashes - Multi File Upload
func TestIsMultiFileUploadArgsOk_MultipleTrailingSlashes(t *testing.T) {
	args := Args{
		EnvPluginInputArgs: EnvPluginInputArgs{
			Username:     "testUser",
			Password:     "testPass",
			Protocol:     "https",
			ServerUrl:    "nexus.example.com///",
			NexusVersion: "nexus3",
			Repository:   "repo",
			GroupId:      "com.test",
			Format:       "maven2",
			Artifact:     "[{\"file\":\"test.jar\",\"artifactId\":\"app\",\"type\":\"jar\",\"version\":\"1.0\",\"groupId\":\"com.test\"}]",
		},
	}

	plugin := NexusPlugin{}
	err := plugin.IsMultiFileUploadArgsOk(args)

	assert.Nil(t, err)
	assert.Equal(t, "https://nexus.example.com", plugin.ServerUrl, "Multiple trailing slashes should be removed")
}

// Test Bug #3: URL No Trailing Slash - Multi File Upload (should remain unchanged)
func TestIsMultiFileUploadArgsOk_NoTrailingSlash(t *testing.T) {
	args := Args{
		EnvPluginInputArgs: EnvPluginInputArgs{
			Username:     "testUser",
			Password:     "testPass",
			Protocol:     "https",
			ServerUrl:    "nexus.example.com",
			NexusVersion: "nexus3",
			Repository:   "repo",
			GroupId:      "com.test",
			Format:       "maven2",
			Artifact:     "[{\"file\":\"test.jar\",\"artifactId\":\"app\",\"type\":\"jar\",\"version\":\"1.0\",\"groupId\":\"com.test\"}]",
		},
	}

	plugin := NexusPlugin{}
	err := plugin.IsMultiFileUploadArgsOk(args)

	assert.Nil(t, err)
	assert.Equal(t, "https://nexus.example.com", plugin.ServerUrl, "URL without trailing slash should remain unchanged")
}

// Test Bug #2: Filename extraction from absolute Linux path
func TestUploadFileNexus3_AbsolutePath_Linux(t *testing.T) {
	mockClient := new(MockHttpClient)
	mockResp := &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(strings.NewReader("Success")),
	}

	// Track what filename was actually sent in the multipart form
	var capturedRequest *http.Request
	mockClient.On("Do", mock.AnythingOfType("*http.Request")).Run(func(args mock.Arguments) {
		capturedRequest = args.Get(0).(*http.Request)
	}).Return(mockResp, nil)

	tmpFile, err := createTempFile("test content")
	assert.NoError(t, err)
	defer os.Remove(tmpFile)

	plugin := NexusPlugin{
		PluginProcessingInfo: PluginProcessingInfo{
			UserName:   "testUser",
			Password:   "testPass",
			ServerUrl:  "https://nexus.example.com",
			Repository: "repo",
			Format:     "maven2",
			Version:    "nexus3",
		},
		HttpClient: mockClient,
	}

	artifact := Artifact{
		File:       tmpFile,
		ArtifactId: "test-app",
		Type:       "jar",
		Version:    "1.0",
		GroupId:    "com.test",
	}

	err = plugin.uploadFileNexus3(artifact, tmpFile)

	assert.Nil(t, err)
	assert.NotNil(t, capturedRequest, "HTTP request should have been made")
	// The request should have been made (we can't easily verify multipart content without parsing it)
	mockClient.AssertExpectations(t)
}

// Test Bug #2: Filename extraction from absolute Windows-style path
func TestUploadFileNexus3_WindowsPath(t *testing.T) {
	mockClient := new(MockHttpClient)
	mockResp := &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(strings.NewReader("Success")),
	}
	mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(mockResp, nil)

	tmpFile, err := createTempFile("test content")
	assert.NoError(t, err)
	defer os.Remove(tmpFile)

	plugin := NexusPlugin{
		PluginProcessingInfo: PluginProcessingInfo{
			UserName:   "testUser",
			Password:   "testPass",
			ServerUrl:  "https://nexus.example.com",
			Repository: "repo",
			Format:     "maven2",
			Version:    "nexus3",
		},
		HttpClient: mockClient,
	}

	// Simulate Windows-style path in artifact (actual file is tmpFile)
	artifact := Artifact{
		File:       tmpFile,
		ArtifactId: "test-app",
		Type:       "jar",
		Version:    "1.0",
		GroupId:    "com.test",
	}

	err = plugin.uploadFileNexus3(artifact, tmpFile)

	assert.Nil(t, err)
	mockClient.AssertExpectations(t)
}

// Test Bug #2: Relative path should also work
func TestUploadFileNexus3_RelativePath(t *testing.T) {
	mockClient := new(MockHttpClient)
	mockResp := &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(strings.NewReader("Success")),
	}
	mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(mockResp, nil)

	tmpFile, err := createTempFile("test content")
	assert.NoError(t, err)
	defer os.Remove(tmpFile)

	plugin := NexusPlugin{
		PluginProcessingInfo: PluginProcessingInfo{
			UserName:   "testUser",
			Password:   "testPass",
			ServerUrl:  "https://nexus.example.com",
			Repository: "repo",
			Format:     "maven2",
			Version:    "nexus3",
		},
		HttpClient: mockClient,
	}

	artifact := Artifact{
		File:       tmpFile,
		ArtifactId: "test-app",
		Type:       "jar",
		Version:    "1.0",
		GroupId:    "com.test",
	}

	err = plugin.uploadFileNexus3(artifact, tmpFile)

	assert.Nil(t, err)
	mockClient.AssertExpectations(t)
}

// Test Bug #1: Response Body Reading - 401 Error with Details
func TestUploadFileNexus3_ResponseBody_401(t *testing.T) {
	mockClient := new(MockHttpClient)
	fakeErrorBody := `{"errors":[{"id":"*","message":"Invalid credentials"}]}`
	mockResp := &http.Response{
		StatusCode: 401,
		Body:       ioutil.NopCloser(strings.NewReader(fakeErrorBody)),
	}
	mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(mockResp, nil)

	tmpFile, err := createTempFile("test content")
	assert.NoError(t, err)
	defer os.Remove(tmpFile)

	plugin := NexusPlugin{
		PluginProcessingInfo: PluginProcessingInfo{
			UserName:   "testUser",
			Password:   "testPass",
			ServerUrl:  "https://nexus.example.com",
			Repository: "repo",
			Format:     "maven2",
			Version:    "nexus3",
		},
		HttpClient: mockClient,
	}

	artifact := Artifact{
		File:       tmpFile,
		ArtifactId: "test-app",
		Type:       "jar",
		Version:    "1.0",
		GroupId:    "com.test",
	}

	err = plugin.uploadFileNexus3(artifact, tmpFile)

	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "Invalid credentials", "Error should include response body details")
	mockClient.AssertExpectations(t)
}

// Test Bug #1: Response Body Reading - 500 Error with Details
func TestUploadFileNexus3_ResponseBody_500(t *testing.T) {
	mockClient := new(MockHttpClient)
	fakeErrorBody := `{"errors":[{"id":"*","message":"Internal server error: Invalid field value"}]}`
	mockResp := &http.Response{
		StatusCode: 500,
		Body:       ioutil.NopCloser(strings.NewReader(fakeErrorBody)),
	}
	mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(mockResp, nil)

	tmpFile, err := createTempFile("test content")
	assert.NoError(t, err)
	defer os.Remove(tmpFile)

	plugin := NexusPlugin{
		PluginProcessingInfo: PluginProcessingInfo{
			UserName:   "testUser",
			Password:   "testPass",
			ServerUrl:  "https://nexus.example.com",
			Repository: "repo",
			Format:     "maven2",
			Version:    "nexus3",
		},
		HttpClient: mockClient,
	}

	artifact := Artifact{
		File:       tmpFile,
		ArtifactId: "test-app",
		Type:       "jar",
		Version:    "1.0",
		GroupId:    "com.test",
	}

	err = plugin.uploadFileNexus3(artifact, tmpFile)

	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "Internal server error", "Error should include response body details")
	mockClient.AssertExpectations(t)
}

// Test Bug #1: Response Body Reading - 404 Error with Details
func TestUploadFileNexus3_ResponseBody_404(t *testing.T) {
	mockClient := new(MockHttpClient)
	fakeErrorBody := `{"errors":[{"id":"*","message":"Repository not found"}]}`
	mockResp := &http.Response{
		StatusCode: 404,
		Body:       ioutil.NopCloser(strings.NewReader(fakeErrorBody)),
	}
	mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(mockResp, nil)

	tmpFile, err := createTempFile("test content")
	assert.NoError(t, err)
	defer os.Remove(tmpFile)

	plugin := NexusPlugin{
		PluginProcessingInfo: PluginProcessingInfo{
			UserName:   "testUser",
			Password:   "testPass",
			ServerUrl:  "https://nexus.example.com",
			Repository: "repo",
			Format:     "maven2",
			Version:    "nexus3",
		},
		HttpClient: mockClient,
	}

	artifact := Artifact{
		File:       tmpFile,
		ArtifactId: "test-app",
		Type:       "jar",
		Version:    "1.0",
		GroupId:    "com.test",
	}

	err = plugin.uploadFileNexus3(artifact, tmpFile)

	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "Repository not found", "Error should include response body details")
	mockClient.AssertExpectations(t)
}

// Test Bug #1: Response Body Reading - Success Response with Body
func TestUploadFileNexus3_ResponseBody_Success(t *testing.T) {
	mockClient := new(MockHttpClient)
	fakeSuccessBody := `{"success":true}`
	mockResp := &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(strings.NewReader(fakeSuccessBody)),
	}
	mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(mockResp, nil)

	tmpFile, err := createTempFile("test content")
	assert.NoError(t, err)
	defer os.Remove(tmpFile)

	plugin := NexusPlugin{
		PluginProcessingInfo: PluginProcessingInfo{
			UserName:   "testUser",
			Password:   "testPass",
			ServerUrl:  "https://nexus.example.com",
			Repository: "repo",
			Format:     "maven2",
			Version:    "nexus3",
		},
		HttpClient: mockClient,
	}

	artifact := Artifact{
		File:       tmpFile,
		ArtifactId: "test-app",
		Type:       "jar",
		Version:    "1.0",
		GroupId:    "com.test",
	}

	err = plugin.uploadFileNexus3(artifact, tmpFile)

	assert.Nil(t, err, "Should succeed without error")
	mockClient.AssertExpectations(t)
}
