package dataimporterspec

// Spec defines the structure of the JSON object passed
// to the import container in the DataImporter Job container.
// This JSON object is mounted as a file (from a Secret) and the file
// path is passed as an argument to the import container.
type Spec struct {
	Source Source         `json:"source"`
	Target Target         `json:"target"`
	Params map[string]any `json:"params,omitempty"`
}

type Source struct {
	Path string    `json:"path,omitempty"`
	S3   *S3Source `json:"s3,omitempty"`
}

type S3Source struct {
	Bucket          string `json:"bucket,omitempty"`
	Region          string `json:"region,omitempty"`
	EndpointURL     string `json:"endpointURL,omitempty"`
	VerifyTLS       bool   `json:"verifyTLS,omitempty"`
	ForcePathStyle  bool   `json:"forcePathStyle,omitempty"`
	AccessKeyID     string `json:"accessKeyID,omitempty"`
	SecretAccessKey string `json:"secretKey,omitempty"`
}

type Target struct {
	Type     string `json:"type,omitempty"`
	Host     string `json:"host,omitempty"`
	Port     string `json:"port,omitempty"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
}
