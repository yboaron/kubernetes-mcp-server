package submariner

import (
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
)

type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return "submariner"
}

func (t *Toolset) GetDescription() string {
	return "Tools for managing and monitoring Submariner multi-cluster connectivity"
}

func (t *Toolset) GetTools(o internalk8s.Openshift) []api.ServerTool {
	return slices.Concat(
		initSubmarinerHealth(),
		initSubmarinerVerify(),
		initSubmarinerFirewall(),
		initSubmarinerESPRemediation(),
	)
}

func init() {
	toolsets.Register(&Toolset{})
}
