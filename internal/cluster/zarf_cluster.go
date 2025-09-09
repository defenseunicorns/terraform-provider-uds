package cluster

import (
	"context"

	zCluster "github.com/zarf-dev/zarf/src/pkg/cluster"
)

// Cluster is dumb
type Cluster interface {
	NewWithWait(ctx context.Context) (*zCluster.Cluster, error)
}
type zarfCluster struct{}

// NewCluster creates a new instance of the Cluster interface
func NewCluster() Cluster {
	return &zarfCluster{}
}

func (z *zarfCluster) NewWithWait(ctx context.Context) (*zCluster.Cluster, error) {
	return zCluster.NewWithWait(ctx)
}
