// Copyright 2020 the Drone Authors. All rights reserved.
// Use of this source code is governed by the Blue Oak Model License
// that can be found in the LICENSE file.

package plugin

import (
	"bytes"
	"context"
	"fmt"
	"gopkg.in/yaml.v2"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"strings"
)

type Plugin interface {
	Init(args *Args) error
	SetBuildRoot(buildRootPath string) error
	DeInit() error
	ValidateAndProcessArgs(args Args) error
	DoPostArgsValidationSetup(args Args) error
	Run() error
	WriteOutputVariables() error
	PersistResults() error
	IsQuiet() bool
	InspectProcessArgs(argNamesList []string) (map[string]interface{}, error)
}

type Args struct {
	EnvPluginInputArgs
	Level string `envconfig:"PLUGIN_LOG_LEVEL"`
}

type EnvPluginInputArgs struct {
	NexusVersion string `envconfig:"PLUGIN_NEXUS_VERSION"`
	Protocol     string `envconfig:"PLUGIN_PROTOCOL"`
	GroupId      string `envconfig:"PLUGIN_GROUP_ID"`
	Repository   string `envconfig:"PLUGIN_REPOSITORY"`
	Artifact     string `envconfig:"PLUGIN_ARTIFACTS"`
	Username     string `envconfig:"PLUGIN_USERNAME"`
	Password     string `envconfig:"PLUGIN_PASSWORD"`

	// For backward compatibility
	ServerUrl  string `envconfig:"PLUGIN_SERVER_URL"`
	Filename   string `envconfig:"PLUGIN_FILENAME"`
	Format     string `envconfig:"PLUGIN_FORMAT"`
	Attributes string `envconfig:"PLUGIN_ATTRIBUTES"`
}

type Artifact struct {
	File       string `yaml:"file"`
	Classifier string `yaml:"classifier"`
	ArtifactId string `yaml:"artifactId"`
	Type       string `yaml:"type"`
	Version    string `yaml:"version"`
	GroupId    string `yaml:"groupId"`
}

func GetNewPlugin(ctx context.Context, args Args) (Plugin, error) {

	nxp := GetNewNexusPlugin()
	return &nxp, nil
}

func Exec(ctx context.Context, args Args) (Plugin, error) {

	plugin, err := GetNewPlugin(ctx, args)
	if err != nil {
		return plugin, err
	}

	err = plugin.Init(&args)
	if err != nil {
		return plugin, err
	}
	defer func(p Plugin) {
		err := p.DeInit()
		if err != nil {
			LogPrintln(p, "Error in DeInit: "+err.Error())
		}
	}(plugin)

	err = plugin.ValidateAndProcessArgs(args)
	if err != nil {
		return plugin, err
	}

	err = plugin.DoPostArgsValidationSetup(args)
	if err != nil {
		return plugin, err
	}

	err = plugin.Run()

	err2 := plugin.WriteOutputVariables()
	if err2 != nil {
		LogPrintln(plugin, "Writing output variable UPLOAD_STATUS failed "+err2.Error())
	}
	if err != nil {
		LogPrintln(plugin, "Upload failed "+err.Error())
		return plugin, err
	}

	err = plugin.PersistResults()
	if err != nil {
		return plugin, err
	}

	return plugin, nil
}

type HttpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type NexusPlugin struct {
	InputArgs         *Args
	IsMultiFileUpload bool
	PluginProcessingInfo
	NexusPluginResponse
	HttpClient HttpClient
}

type PluginProcessingInfo struct {
	UserName   string
	Password   string
	ServerUrl  string
	Version    string
	Format     string
	Repository string
	GroupId    string
	Artifacts  []Artifact
}

type NexusPluginResponse struct {
	Failed []FailedArtifact `json:"failed"`
}

type FailedArtifact struct {
	File       string `json:"file"`
	ArtifactId string `json:"artifactId"`
	Err        string `json:"err"`
}

func (n *NexusPlugin) Run() error {
	LogPrintln(n, "Starting Nexus Plugin Run")

	if n.HttpClient == nil {
		n.HttpClient = &http.Client{}
	}

	for _, artifact := range n.Artifacts {
		filePath := artifact.File
		file, err := os.Open(filePath)
		if err != nil {
			n.addFailedArtifact(artifact, fmt.Sprintf("could not open file: %v", err))
			continue
		}

		if n.Version == "nexus2" {
			artifactURL := n.prepareNexus2ArtifactURL(artifact)
			if err := n.uploadFileNexus2(artifactURL, file, filePath); err != nil {
				n.addFailedArtifact(artifact, fmt.Sprintf("upload failed: %v", err))
				err := file.Close()
				if err != nil {
					LogPrintln(n, "Error closing file: ", err.Error())
				}
				continue
			}
		} else if n.Version == "nexus3" {
			if err := n.uploadFileNexus3(artifact, filePath); err != nil {
				n.addFailedArtifact(artifact, fmt.Sprintf("upload failed: %v", err))
				err := file.Close()
				if err != nil {
					LogPrintln(n, "Error closing file: ", err.Error())
				}
				continue
			}
		}
		err = file.Close()
		if err != nil {
			LogPrintln(n, "Error closing file: ", err.Error())
		}

		fmt.Println("Successfully uploaded artifact:", filePath)
	}

	if len(n.Failed) > 0 {
		return GetNewError("NexusPlugin Error in Run: some artifacts failed to upload")
	}

	return nil
}

func (n *NexusPlugin) WriteOutputVariables() error {

	type EnvKvPair struct {
		Key   string
		Value interface{}
	}
	var kvPairs []EnvKvPair

	if len(n.Failed) == 0 {
		LogPrintln(n, "All artifacts uploaded successfully")
		kvPairs = append(kvPairs, EnvKvPair{Key: "UPLOAD_STATUS", Value: "Success"})
	} else {
		kvPairs = append(kvPairs, EnvKvPair{Key: "UPLOAD_STATUS", Value: n.Failed})
	}

	var retErr error = nil

	for _, kvPair := range kvPairs {
		err := WriteEnvVariableAsString(kvPair.Key, kvPair.Value)
		if err != nil {
			retErr = err
		}
	}

	return retErr
}

func (n *NexusPlugin) Init(args *Args) error {
	n.InputArgs = args
	return nil
}

func (n *NexusPlugin) SetBuildRoot(buildRootPath string) error {
	return nil
}

func (n *NexusPlugin) DeInit() error {
	return nil
}

func (n *NexusPlugin) ValidateAndProcessArgs(args Args) error {
	LogPrintln(n, "NexusPlugin BuildAndValidateArgs")

	err := n.DetermineIsMultiFileUpload(args)
	if err != nil {
		LogPrintln(n, "NexusPlugin Error in ValidateAndProcessArgs: "+err.Error())
		return err
	}

	if n.IsMultiFileUpload {
		err = n.IsMultiFileUploadArgsOk(args)
		if err != nil {
			LogPrintln(n, "NexusPlugin Error in ValidateAndProcessArgs: "+err.Error())
			return err
		}
	} else {
		err = n.IsSingleFileUploadArgsOk(args)
		if err != nil {
			LogPrintln(n, "NexusPlugin Error in ValidateAndProcessArgs: "+err.Error())
			return err
		}
	}

	return nil
}

func (n *NexusPlugin) DetermineIsMultiFileUpload(args Args) error {
	LogPrintln(n, "NexusPlugin DetermineIsMultiFileUpload")

	switch {
	case args.Attributes != "" && args.Artifact == "":
		n.IsMultiFileUpload = false
	case args.Artifact != "" && args.Attributes == "":
		n.IsMultiFileUpload = true
	case args.Attributes == "" && args.Artifact == "":
		return GetNewError("Error in DetermineCompatibilityMode: both 'Attributes' and 'Artifact' cannot be empty")
	default:
		return GetNewError("Error in DetermineCompatibilityMode: both 'Attributes' and 'Artifact' provided, which is ambiguous")
	}

	return nil
}

func (n *NexusPlugin) IsMultiFileUploadArgsOk(args Args) error {
	LogPrintln(n, "NexusPlugin IsMultiFileUploadArgsOk")

	requiredArgs := map[string]string{
		"username":     args.Username,
		"password":     args.Password,
		"protocol":     args.Protocol,
		"nexusUrl":     args.ServerUrl,
		"nexusVersion": args.NexusVersion,
		"repository":   args.Repository,
		"groupId":      args.GroupId,
		"format":       args.Format,
	}

	for field, value := range requiredArgs {
		if value == "" {
			return GetNewError("Error in IsMultiFileUploadArgsOk: " + field + " cannot be empty")
		}
	}

	n.UserName = args.Username
	n.Password = args.Password
	n.Repository = args.Repository
	n.ServerUrl = args.Protocol + "://" + args.ServerUrl
	n.GroupId = args.GroupId
	n.Version = args.NexusVersion
	n.Format = args.Format

	// Unmarshalling YAML artifact data
	var artifacts []Artifact
	if err := yaml.Unmarshal([]byte(args.Artifact), &artifacts); err != nil {
		return GetNewError("Error in IsMultiFileUploadArgsOk: Error decoding YAML: " + err.Error())
	}

	var filteredArtifacts []Artifact
	for _, artifact := range artifacts {
		missingFields := []string{}
		if artifact.ArtifactId == "" {
			missingFields = append(missingFields, "ArtifactId")
		}
		if artifact.File == "" {
			missingFields = append(missingFields, "File")
		}
		if artifact.Type == "" {
			missingFields = append(missingFields, "Type")
		}
		if artifact.Version == "" {
			missingFields = append(missingFields, "Version")
		}
		if artifact.GroupId == "" {
			artifact.GroupId = args.GroupId
		}
		if len(missingFields) > 0 {
			n.addFailedArtifact(artifact, fmt.Sprintf("Missing fields: %s", strings.Join(missingFields, ", ")))
		} else {
			// Add to filtered list if all fields are valid
			filteredArtifacts = append(filteredArtifacts, artifact)
		}
	}

	n.Artifacts = filteredArtifacts
	return nil
}

func (n *NexusPlugin) IsSingleFileUploadArgsOk(args Args) error {
	LogPrintln(n, "NexusPlugin IsSingleFileUploadArgsOk")

	requiredArgs := map[string]string{
		"Username":   args.Username,
		"Password":   args.Password,
		"ServerUrl":  args.ServerUrl,
		"Filename":   args.Filename,
		"Format":     args.Format,
		"Repository": args.Repository,
	}

	for field, value := range requiredArgs {
		if value == "" {
			return GetNewError("Error in IsSingleFileUploadArgsOk: " + field + " cannot be empty")
		}
	}

	requiredFields := []string{"CgroupId", "Cversion", "Aextension", "Aclassifier"}
	values := make(map[string]string)

	pattern := regexp.MustCompile(`-(CgroupId|CartifactId|Cversion|Aextension|Aclassifier)=(\S+)`)
	matches := pattern.FindAllStringSubmatch(args.Attributes, -1)

	for _, match := range matches {
		if len(match) == 3 {
			values[match[1]] = match[2]
		}
	}

	// Check if all required fields are present
	for _, field := range requiredFields {
		if values[field] == "" {
			return GetNewError("Error in IsSingleFileUploadArgsOk: " + field + " cannot be empty")
		}
	}
	n.UserName = args.Username
	n.Password = args.Password
	n.Repository = args.Repository
	n.ServerUrl = args.ServerUrl
	n.Format = args.Format
	n.GroupId = values["CgroupId"]
	n.Version = "nexus3"
	n.Artifacts = []Artifact{
		{
			File:       args.Filename,
			Classifier: values["Aclassifier"],
			ArtifactId: values["CartifactId"],
			Type:       values["Aextension"],
			Version:    values["Cversion"],
			GroupId:    values["CgroupId"],
		},
	}

	return nil
}

func (n *NexusPlugin) DoPostArgsValidationSetup(args Args) error {
	return nil
}

func (n *NexusPlugin) PersistResults() error {
	return nil
}

func (n *NexusPlugin) IsQuiet() bool {
	return false
}

func (n *NexusPlugin) InspectProcessArgs(argNamesList []string) (map[string]interface{}, error) {
	return nil, nil
}

func GetNewNexusPlugin() NexusPlugin {
	return NexusPlugin{}
}

func (n *NexusPlugin) prepareNexus2ArtifactURL(artifact Artifact) string {
	switch n.Format {
	case "maven2":
		return fmt.Sprintf("%s/repository/%s/%s/%s/%s/%s-%s.%s",
			n.ServerUrl, n.Repository, artifact.GroupId, artifact.ArtifactId, artifact.Version,
			artifact.ArtifactId, artifact.Version, artifact.Type)

	case "yum":
		return fmt.Sprintf("%s/repository/%s/%s/%s",
			n.ServerUrl, n.Repository, artifact.ArtifactId, artifact.Version)

	case "raw":
		return fmt.Sprintf("%s/repository/%s/%s/%s.%s",
			n.ServerUrl, n.Repository, artifact.GroupId, artifact.ArtifactId, artifact.Type)

	default:
		LogPrintln(n, "Unsupported format for direct upload:", n.Format)
		return ""
	}
}

func (n *NexusPlugin) uploadFileNexus2(url string, content io.Reader, filePath string) error {
	req, err := http.NewRequest("PUT", url, content)
	if err != nil {
		return err
	}

	req.SetBasicAuth(n.UserName, n.Password)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := n.HttpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		fmt.Println("File upload failed status ", resp.StatusCode)
		return fmt.Errorf("Upload failed with status %d", resp.StatusCode)
	}
	return nil
}

func (n *NexusPlugin) uploadFileNexus3(artifact Artifact, filePath string) error {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	var url string
	var assetFieldName string

	switch n.Format {
	case "maven2":
		_ = writer.WriteField("maven2.groupId", artifact.GroupId)
		_ = writer.WriteField("maven2.artifactId", artifact.ArtifactId)
		_ = writer.WriteField("maven2.version", artifact.Version)
		assetFieldName = "maven2.asset1"
		_ = writer.WriteField("maven2.asset1.extension", artifact.Type)

	case "raw":
		_ = writer.WriteField("raw.directory", artifact.GroupId)
		assetFieldName = "raw.asset1"
		_ = writer.WriteField("raw.asset1.filename", fmt.Sprintf("%s.%s", artifact.ArtifactId, artifact.Type))

	default:
		assetFieldName = fmt.Sprintf("%s.asset", n.Format)
	}

	fileWriter, err := writer.CreateFormFile(assetFieldName, artifact.File)
	if err != nil {
		LogPrintln(n, "Error CreateFormFile: ", err.Error())
		return err
	}
	file, err := os.Open(artifact.File)
	if err != nil {
		LogPrintln(n, "Error os.Open(artifact.File): ", err.Error())
		return err
	}
	defer file.Close()
	_, err = io.Copy(fileWriter, file)
	if err != nil {
		LogPrintln(n, "Error io.Copy(fileWriter, file): ", err.Error())
		return err
	}

	err = writer.Close()
	if err != nil {
		LogPrintln(n, "Error writer.Close(): ", err.Error())
		return err
	}

	url = fmt.Sprintf("%s/service/rest/v1/components?repository=%s", n.ServerUrl, n.Repository)

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		LogPrintln(n, "Error http.NewRequest: ", err.Error())
		return err
	}

	req.SetBasicAuth(n.UserName, n.Password)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := n.HttpClient.Do(req)
	if err != nil {
		LogPrintln(n, "Error n.HttpClient.Do(req): ", err.Error())
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		LogPrintln(n, "Error upload failed with status: ", resp.StatusCode)
		return fmt.Errorf("Upload failed with status %d", resp.StatusCode)
	}

	return nil
}

func (n *NexusPlugin) addFailedArtifact(artifact Artifact, errMsg string) {
	n.Failed = append(n.Failed, FailedArtifact{
		File:       artifact.File,
		ArtifactId: artifact.ArtifactId,
		Err:        errMsg,
	})
}
