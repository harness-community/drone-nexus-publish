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
