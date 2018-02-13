package driver

type Status string
type Bool string

const (
	StatusSuccess      Status = "Success"
	StatusFailure      Status = "Failure"
	StatusNotSupported Status = "Not Supported"
	BoolTrue           Bool   = "True"
	BoolFalse          Bool   = "False"
)

type FlexVolume interface {
	Init() InitResponse
	Attach(map[string]string, string) AttachResponse
	Detach(string, string) CommonResponse
	WaitForAttach(string, map[string]string) AttachResponse
	IsAttached(map[string]string, string) IsAttachedResponse
	Mount(string, string, map[string]string) CommonResponse
	Unmount(string) CommonResponse
}

type CommonResponse struct {
	Status  Status `json:"status"`
	Message string `json:"message"`
}

type AttachResponse struct {
	CommonResponse
	Device string `json:"device"`
}

type IsAttachedResponse struct {
	CommonResponse
	Attached Bool `json:"attached"`
}

type InitResponse struct {
	CommonResponse
	Capabilities struct {
		Attach bool `json:"attach"`
	} `json:"capabilities"`
}
