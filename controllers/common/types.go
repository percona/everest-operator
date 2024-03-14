package common

import "errors"

// ClusterType represents the type of the cluster.
type ClusterType string

var (
	ErrPitrTypeIsNotSupported = errors.New("unknown PITR type")
	ErrPitrTypeLatest         = errors.New("'latest' type is not supported by Everest yet")
	ErrPitrEmptyDate          = errors.New("no date provided for PITR of type 'date'")

	ErrPSMDBOneStorageRestriction = errors.New("using more than one storage is not allowed for psmdb clusters")
)
