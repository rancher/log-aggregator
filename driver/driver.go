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
	"github.com/urfave/cli"

	"github.com/rancher/log-aggregator/generator"
)

const (
	mountCmd               = "mount"
	unmountCmd             = "umount"
	svcLogBaseDir          = "/var/log/rancher-log-volumes"
	svcProjectLogConfigDir = "/var/lib/fluentd/etc/config/customer/project"
	svcClusterLogConfigDir = "/var/lib/fluentd/etc/config/customer/cluster"
)

const (
	tmpClusterDir = "/tmp/fluentd/etc/config/customer/cluster"
	tmpProjectDir = "/tmp/fluentd/etc/config/customer/project"
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
	if err = checkArgsLen(args, 2); err != nil {
		return returnErrorResponse(err)
	}

	containerPath := args[0]
	opts := Options{}
	if err = json.Unmarshal([]byte(args[1]), &opts); err != nil {
		return returnErrorResponse(err)
	}

	if _, err = valid.ValidateStruct(opts); err != nil {
		return returnErrorResponse(err)
	}

	//generate config
	if err := precreateDir(); err != nil {
		return returnErrorResponse(err)
	}

	fn := []string{opts.ClusterID, opts.ClusterName, opts.Namespace, opts.ProjectID, opts.ProjectName, opts.WorkloadName, opts.ContainerName, opts.VolumeName}
	generateDir := strings.Join(fn, "_")

	var hostDir string
	if isContain(opts.Format, predefineFormat) {
		hostDir = path.Join(svcLogBaseDir, opts.Format, generateDir)
	} else {
		hostDir = path.Join(svcLogBaseDir, customiseFormat, generateDir)
		if err = generateCustomiseConfig(generateDir, hostDir, opts); err != nil {
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
	if err = checkArgsLen(args, 1); err != nil {
		return returnErrorResponse(err)
	}

	containerPath := args[0]
	if err = unMount(containerPath); err != nil {
		return returnErrorResponse(fmt.Errorf("unmount container path %s failed, %v", containerPath, err))
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
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf(string(output))
	}
	return nil
}

func checkArgsLen(args cli.Args, expectedNum int) error {
	if len(args) < expectedNum {
		err := fmt.Errorf("mount: invalid args num, %v", args)
		return err
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

func generateCustomiseConfig(generateDir, hostDir string, opts Options) error {
	var err error
	outputProjectPath := path.Join(svcProjectLogConfigDir, generateDir+".conf")
	outputClusterPath := path.Join(svcClusterLogConfigDir, generateDir+".conf")
	conf := map[string]interface{}{
		"Format":         opts.Format,
		"Path":           fmt.Sprintf("%s/*.*", hostDir),
		"Project":        fmt.Sprintf("%s:%s", opts.ClusterID, opts.ProjectID),
		"ClusterPosPath": fmt.Sprintf("/fluentd/etc/log/custom_cluster_userformat_%s.pos", generateDir),
		"ProjectPosPath": fmt.Sprintf("/fluentd/etc/log/custom_project_userformat_%s.pos", generateDir),
	}

	tmpClusterConfigFile := path.Join(tmpClusterDir, generateDir+".conf")
	if err = generator.GenerateConfigFile(tmpClusterConfigFile, generator.ClusterSourceTemplate, "cluster", conf); err != nil {
		return fmt.Errorf("generate cluster config file failed, %v", err)
	}

	if err = isConfigEqual(tmpClusterConfigFile, outputClusterPath); err != nil {
		if err = copyFileContent(tmpClusterConfigFile, outputClusterPath); err != nil {
			return err
		}
	}

	tmpProjectConfigFile := path.Join(tmpProjectDir, generateDir+".conf")
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
