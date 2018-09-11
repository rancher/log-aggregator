package driver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"

	valid "github.com/asaskevich/govalidator"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/rancher/log-aggregator/generator"
)

const (
	mountCmd               = "mount"
	unmountCmd             = "umount"
	svcLogBaseDir          = "/var/lib/rancher/log-volumes"
	svcLogPosDir           = "/var/lib/rancher/fluentd/log"
	svcProjectLogConfigDir = "/var/lib/rancher/fluentd/etc/config/custom/project"
	svcClusterLogConfigDir = "/var/lib/rancher/fluentd/etc/config/custom/cluster"
)

const (
	tmpClusterDir        = "/tmp/fluentd/etc/config/custom/cluster"
	tmpProjectDir        = "/tmp/fluentd/etc/config/custom/project"
	clusterPosFilePrefix = "custom_cluster_userformat_"
	projectPosFilePrefix = "custom_project_userformat_"
)

var (
	predefineFormat = []string{"json", "apache2", "nginx", "rfc3164", "rfc5424"}
	customiseFormat = "customise"
)

var (
	mountCmdArg = []string{"-o", "bind"}
)

type Options struct {
	ClusterName   string `json:"clusterName,omitempty" valid:"required"`
	ClusterID     string `json:"clusterID,omitempty" valid:"required"`
	ProjectName   string `json:"projectName,omitempty" valid:"required"`
	ProjectID     string `json:"projectID,omitempty" valid:"required"`
	Namespace     string `json:"namespace,omitempty" valid:"required"`
	WorkloadName  string `json:"workloadName,omitempty" valid:"required"`
	ContainerName string `json:"containerName,omitempty" valid:"required"`
	Format        string `json:"format,omitempty" valid:"required"`
	VolumeName    string `json:"volumeName,omitempty" valid:"required"`
	PodName       string `json:"kubernetes.io/pod.name,omitempty" valid:"required"`
	PodUID        string `json:"kubernetes.io/pod.uid,omitempty" valid:"required"`
}

type FlexVolumeDriver struct {
	Logger *logrus.Logger
}

func (f *FlexVolumeDriver) Init() InitResponse {
	if err := precreateDir(); err != nil {
		return InitResponse{
			CommonResponse: returnErrorResponse(err),
		}
	}

	if err := os.MkdirAll(tmpProjectDir, os.ModePerm); err != nil {
		return InitResponse{
			CommonResponse: returnErrorResponse(fmt.Errorf("create tmp dir %s failed, %v", tmpProjectDir, err)),
		}
	}

	if err := os.MkdirAll(svcProjectLogConfigDir, os.ModePerm); err != nil {
		return InitResponse{
			CommonResponse: returnErrorResponse(fmt.Errorf("create config dir %s failed, %v", svcProjectLogConfigDir, err)),
		}
	}

	if err := os.MkdirAll(svcClusterLogConfigDir, os.ModePerm); err != nil {
		return InitResponse{
			CommonResponse: returnErrorResponse(fmt.Errorf("create config dir %s failed, %v", svcClusterLogConfigDir, err)),
		}
	}

	if err := os.MkdirAll(svcLogBaseDir, os.ModePerm); err != nil {
		return InitResponse{
			CommonResponse: returnErrorResponse(fmt.Errorf("create log dir %s failed, %v", svcLogBaseDir, err)),
		}
	}

	return InitResponse{
		CommonResponse: CommonResponse{
			Status:  StatusSuccess,
			Message: "Success",
		},
		Capabilities: struct {
			Attach bool `json:"attach"`
		}{
			Attach: false,
		},
	}
}

func (f *FlexVolumeDriver) Mount(args []string) CommonResponse {
	var err error
	defer func(logger *logrus.Logger) {
		if err != nil {
			logger.Error(err)
		}
	}(f.Logger)
	// param check
	f.Logger.Debugf("mount args: %v", args)
	switch len(args) {
	case 2:
	case 3:
		logrus.Warnf("flex volume from kubelet is standard, args length is %d", len(args))
	default:
		return returnErrorResponse(fmt.Errorf("mount: invalid args num, %v", args))
	}

	containerPath := args[0]
	opts := Options{}
	if err = json.Unmarshal([]byte(args[len(args)-1]), &opts); err != nil {
		return returnErrorResponse(err)
	}

	if _, err = valid.ValidateStruct(opts); err != nil {
		return returnErrorResponse(err)
	}
	formatOption(&opts)

	//generate config
	if err := precreateDir(); err != nil {
		return returnErrorResponse(err)
	}

	fn := []string{opts.ClusterID, opts.ClusterName, opts.Namespace, opts.ProjectID, opts.ProjectName, opts.WorkloadName, opts.PodName, opts.ContainerName}
	generateDir := strings.Join(fn, "_")
	identifyDir := fmt.Sprintf("%s_%s", opts.PodUID, opts.VolumeName)

	var hostDir string
	if isContain(opts.Format, predefineFormat) {
		hostDir = path.Join(svcLogBaseDir, identifyDir, opts.Format, generateDir)
	} else {
		hostDir = path.Join(svcLogBaseDir, identifyDir, customiseFormat, generateDir)
		if err = generateCustomiseConfig(hostDir, opts); err != nil {
			f.Logger.Error(err)
			returnErrorResponse(err)
		}
	}

	if err = os.MkdirAll(hostDir, os.ModePerm); err != nil {
		return returnErrorResponse(fmt.Errorf("create hostPath failed, %v", err))
	}

	if err = bindMount(hostDir, containerPath); err != nil {
		return returnErrorResponse(fmt.Errorf("bind mount failed, %v", err))
	}

	return CommonResponse{
		Status:  StatusSuccess,
		Message: "Success",
	}
}

func (f *FlexVolumeDriver) Unmount(args []string) CommonResponse {
	var err error
	defer func(logger *logrus.Logger) {
		if err != nil {
			logger.Error(err)
		}
	}(f.Logger)

	f.Logger.Debugf("ummount args: %v", args)
	if len(args) != 1 {
		return returnErrorResponse(fmt.Errorf("unmount: invalid args num, %v", args))
	}

	containerPath := args[0]
	if err = unMount(containerPath); err != nil {
		return returnErrorResponse(fmt.Errorf("unmount container path %s failed, %v", containerPath, err))
	}
	strArray := strings.Split(containerPath, "/")
	var podUID string
	for i, v := range strArray { //example v: /var/lib/kubelet/pods/be6a7bc3-b278-11e8-973b-08002749a29c/volumes/cattle.io~log-aggregator/vol1
		if v == "pods" {
			podUID = strArray[i+1]
			break
		}
	}
	// clean up
	volumeName := strArray[len(strArray)-1]
	identifyName := fmt.Sprintf("%s_%s", podUID, volumeName)

	configFiles := []string{fmt.Sprintf("%s/%s.conf", svcClusterLogConfigDir, identifyName), fmt.Sprintf("%s/%s.conf", svcProjectLogConfigDir, identifyName)}
	if err = removeFiles(configFiles); err != nil {
		returnErrorResponse(fmt.Errorf("remove custom config files %v failed", configFiles))
	}

	mountPoint := []string{path.Join(svcLogBaseDir, identifyName)}
	if err = removeFiles(mountPoint); err != nil {
		returnErrorResponse(fmt.Errorf("remove custom mount point %v failed", mountPoint))
	}

	posFiles := []string{fmt.Sprintf("%s/%s%s.pos", svcLogPosDir, clusterPosFilePrefix, identifyName), fmt.Sprintf("%s/%s%s.pos", svcLogPosDir, projectPosFilePrefix, identifyName)}
	if err = removeFiles(posFiles); err != nil {
		returnErrorResponse(fmt.Errorf("remove custom pos files %v failed", posFiles))
	}

	return CommonResponse{
		Status:  StatusSuccess,
		Message: "Success",
	}
}

func bindMount(hostPath string, containerPath string) error {
	c := append(mountCmdArg, hostPath)
	c = append(c, containerPath)
	cmd := exec.Command(mountCmd, c...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("run bind mount command failed, hostPath: %s, containerPath: %s, error: %v, output: %s", hostPath, containerPath, err, string(output))
	}
	return nil
}

func unMount(containerPath string) error {
	cmd := exec.Command(unmountCmd, containerPath)
	if output, err := cmd.CombinedOutput(); err != nil && !(strings.Contains(string(output), "not mounted") || strings.Contains(string(output), "mountpoint not found")) {
		return fmt.Errorf(string(output))
	}
	return nil
}

func returnErrorResponse(err error) CommonResponse {
	return CommonResponse{
		Status:  StatusFailure,
		Message: fmt.Sprintf("%v", err),
	}
}

func isConfigEqual(file1, file2 string) error {
	f1, err := ioutil.ReadFile(file1)
	if err != nil {
		return errors.Wrapf(err, "fail read file %s", file1)
	}

	f2, err := ioutil.ReadFile(file2)

	if err != nil {
		return errors.Wrapf(err, "fail read file %s", file2)
	}
	if bytes.Equal(f1, f2) {
		return nil
	}
	return fmt.Errorf("file not equal")
}

func copyFileContent(fromPath, toPath string) error {
	from, err := os.Open(fromPath)
	if err != nil {
		return errors.Wrapf(err, "fail to open tmp config file")
	}
	defer from.Close()

	to, err := os.OpenFile(toPath, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return errors.Wrap(err, "fail to open current config file")
	}
	defer to.Close()

	_, err = io.Copy(to, from)
	if err != nil {
		return errors.Wrap(err, "fail to copy config file")
	}
	if err = to.Sync(); err != nil {
		return errors.Wrap(err, "fail to sync config file")
	}
	return nil
}

func precreateDir() error {
	if err := os.MkdirAll(tmpClusterDir, os.ModePerm); err != nil {
		return fmt.Errorf("create tmp dir %s failed, %v", tmpClusterDir, err)
	}

	if err := os.MkdirAll(tmpProjectDir, os.ModePerm); err != nil {
		return fmt.Errorf("create tmp dir %s failed, %v", tmpProjectDir, err)
	}

	if err := os.MkdirAll(svcProjectLogConfigDir, os.ModePerm); err != nil {
		return fmt.Errorf("create config dir %s failed, %v", svcProjectLogConfigDir, err)
	}

	if err := os.MkdirAll(svcClusterLogConfigDir, os.ModePerm); err != nil {
		return fmt.Errorf("create config dir %s failed, %v", svcClusterLogConfigDir, err)
	}

	if err := os.MkdirAll(svcLogBaseDir, os.ModePerm); err != nil {
		return fmt.Errorf("create log dir %s failed, %v", svcLogBaseDir, err)
	}
	return nil
}

func removeFiles(files []string) error {
	for _, v := range files {
		fileInfo, err := os.Stat(v)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}

		if fileInfo.IsDir() {
			err := os.RemoveAll(v)
			return errors.Wrapf(err, "remove dir %s failed", v)
		}

		if err := os.Remove(v); err != nil {
			return errors.Wrapf(err, "remove file %s failed", v)
		}
	}
	return nil
}

func isContain(obj string, target []string) bool {
	for _, v := range target {
		if obj == v {
			return true
		}
	}
	return false
}

func generateCustomiseConfig(hostDir string, opts Options) error {
	var err error
	configFileName := fmt.Sprintf("%s_%s.conf", opts.PodUID, opts.VolumeName)
	outputProjectPath := path.Join(svcProjectLogConfigDir, configFileName)
	outputClusterPath := path.Join(svcClusterLogConfigDir, configFileName)
	conf := map[string]interface{}{
		"Format":         opts.Format,
		"Path":           fmt.Sprintf("%s/*.*", hostDir),
		"ClusterPosPath": fmt.Sprintf("/fluentd/log/%s%s_%s.pos", clusterPosFilePrefix, opts.PodUID, opts.VolumeName),
		"ProjectPosPath": fmt.Sprintf("/fluentd/log/%s%s_%s.pos", projectPosFilePrefix, opts.PodUID, opts.VolumeName),
	}

	tmpClusterConfigFile := path.Join(tmpClusterDir, configFileName)
	if err = generator.GenerateConfigFile(tmpClusterConfigFile, generator.ClusterSourceTemplate, "cluster", conf); err != nil {
		return fmt.Errorf("generate cluster config file failed, %v", err)
	}

	if err = isConfigEqual(tmpClusterConfigFile, outputClusterPath); err != nil {
		if err = copyFileContent(tmpClusterConfigFile, outputClusterPath); err != nil {
			return err
		}
	}

	tmpProjectConfigFile := path.Join(tmpProjectDir, configFileName)
	if err = generator.GenerateConfigFile(tmpProjectConfigFile, generator.ProjectSourceTemplate, "project", conf); err != nil {
		return fmt.Errorf("generate project config file failed, %v", err)
	}

	if err = isConfigEqual(tmpProjectConfigFile, outputProjectPath); err != nil {
		if err = copyFileContent(tmpProjectConfigFile, outputProjectPath); err != nil {
			return err
		}
	}

	return removeFiles([]string{tmpClusterConfigFile, tmpProjectConfigFile})
}

func formatOption(opts *Options) {
	opts.ProjectName = strings.Replace(opts.ProjectName, "_", "~", -1)
}
