package dataimporterspec

// The following environment variables are available to the DataImporter Job container.
const (
	EnvS3AccessKeyID     = "S3_ACCESS_KEY_ID"
	EnvS3SecretAccessKey = "S3_SECRET_ACCESS_KEY"
	EnvDBUsername        = "DB_USERNAME"
	EnvDBPassword        = "DB_PASSWORD"
)

// Spec defines the structure of the JSON object passed
// to the DataImporter Job container.
// This object is passed via stdin.
type Spec struct {
	Source Source         `json:"source"`
	Target Target         `json:"target"`
	Params map[string]any `json:"params,omitempty"`
}

type Source struct {
	S3 *S3Source `json:"s3,omitempty"`
}

type S3Source struct {
	Bucket         string `json:"bucket,omitempty"`
	Region         string `json:"region,omitempty"`
	EndpointURL    string `json:"endpointURL,omitempty"`
	VerifyTLS      bool   `json:"verifyTLS,omitempty"`
	ForcePathStyle bool   `json:"forcePathStyle,omitempty"`
}

type Target struct {
	Type string `json:"type,omitempty"`
	Host string `json:"host,omitempty"`
	Port int    `json:"port,omitempty"`
}
