package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type clusterSummary struct {
	ClusterID       json.Number `json:"clusterId"`
	ClusterName     string      `json:"clusterName"`
	PrimaryServer   string      `json:"primaryServer"`
	SecondaryServer string      `json:"secondaryServer"`
	RcpPath         string      `json:"rcpPath"`
	RcpSudoWrapper  bool        `json:"rcpSudoWrapper"`
	RepositoryType  string      `json:"repositoryType"`
	RshPath         string      `json:"rshPath"`
	RshSudoWrapper  bool        `json:"rshSudoWrapper"`
	UIDDomain       string      `json:"uidDomain"`
}

type clusterInfo struct {
	Cluster struct {
		ClusterSummary clusterSummary `json:"clusterSummary"`
	} `json:"cluster"`
}

type nodeInfo struct {
	NodeNumber    int    `json:"nodeNumber"`
	AdminNodeName string `json:"adminNodeName"`
	Config        struct {
		AdminLoginName string `json:"adminLoginName"`
	} `json:"config"`
	Network struct {
		DaemonIPAddress string `json:"daemonIPAddress"`
		DaemonNodeName  string `json:"daemonNodeName"`
	} `json:"network"`
	Roles struct {
		Designation    string `json:"designation"`
		OtherNodeRoles string `json:"otherNodeRoles"`
	} `json:"roles"`
}

func cmdMmlscluster() *command {
	return &command{
		name:    "mmlscluster",
		summary: "Display cluster configuration",
		usage: `Usage:
  mmlscluster [-Y]`,
		spec: optSpec{"-Y": flagOpt},
		run:  runMmlscluster,
	}
}

func runMmlscluster(o options) error {
	if len(o.operands) > 0 {
		return usagef("Incorrect operand: %s", o.operands[0])
	}

	var cluster clusterInfo
	if err := scaleGetInto("cluster", nil, &cluster); err != nil {
		return err
	}
	var nodes struct {
		Nodes []nodeInfo `json:"nodes"`
	}
	if err := scaleGetInto("nodes", queryAllFields(), &nodes); err != nil {
		return err
	}

	summary := cluster.Cluster.ClusterSummary
	if o.has("-Y") {
		printClusterY(summary, nodes.Nodes)
		return nil
	}

	fmt.Println()
	fmt.Println("GPFS cluster information")
	fmt.Println("========================")
	fmt.Printf("  %-27s%s\n", "GPFS cluster name:", summary.ClusterName)
	fmt.Printf("  %-27s%s\n", "GPFS cluster id:", summary.ClusterID.String())
	fmt.Printf("  %-27s%s\n", "GPFS UID domain:", summary.UIDDomain)
	fmt.Printf("  %-27s%s\n", "Remote shell command:", summary.RshPath)
	fmt.Printf("  %-27s%s\n", "Remote file copy command:", summary.RcpPath)
	fmt.Printf("  %-27s%s\n", "Repository type:", summary.RepositoryType)
	fmt.Println()

	// Column widths follow the longest value, as mmlscluster's do.
	daemonWidth := columnWidth("Daemon node name", nodes.Nodes, func(n nodeInfo) string { return n.Network.DaemonNodeName })
	ipWidth := columnWidth("IP address", nodes.Nodes, func(n nodeInfo) string { return n.Network.DaemonIPAddress })
	adminWidth := columnWidth("Admin node name", nodes.Nodes, func(n nodeInfo) string { return n.AdminNodeName })

	header := fmt.Sprintf("%5s  %-*s%-*s%-*sDesignation", "Node",
		daemonWidth, "Daemon node name", ipWidth, "IP address", adminWidth, "Admin node name")
	fmt.Println(header)
	fmt.Println(strings.Repeat("-", len(header)+1))
	for _, node := range nodes.Nodes {
		fmt.Printf("%4d   %-*s%-*s%-*s%s\n", node.NodeNumber,
			daemonWidth, node.Network.DaemonNodeName,
			ipWidth, node.Network.DaemonIPAddress,
			adminWidth, node.AdminNodeName,
			nodeDesignation(node))
	}
	fmt.Println()
	return nil
}

// printClusterY writes mmlscluster's -Y records. mmlscluster announces every
// section it knows before printing any row, including the CNFS, CES, cloud
// gateway and comment sections that carry no rows on a cluster without those
// features, so consumers see the same record set here.
func printClusterY(summary clusterSummary, nodes []nodeInfo) {
	clusterRecord := newYRecord("mmlscluster", "clusterSummary",
		"clusterName", "clusterId", "uidDomain", "rshPath", "rshSudoWrapper",
		"rcpPath", "rcpSudoWrapper", "repositoryType", "primaryServer", "secondaryServer")
	nodeRecord := newYRecord("mmlscluster", "clusterNode",
		"nodeNumber", "daemonNodeName", "ipAddress", "adminNodeName", "designation",
		"otherNodeRoles", "adminLoginName", "otherNodeRolesAlias")
	commentRecord := newYRecord("mmlscluster", "commentNode",
		"nodeNumber", "daemonNodeName", "comment_enc")

	clusterRecord.printHeader()
	nodeRecord.printHeader()
	newYRecord("mmlscluster", "cnfsSummary", "cnfsSharedRoot", "cnfsMoundPort",
		"cnfsNFSDprocs", "cnfsReboot", "cnfsMonitorEnabled", "cnfsGanesha").printHeader()
	newYRecord("mmlscluster", "cnfsNode", "nodeNumber", "daemonNodeName", "ipAddress",
		"cnfsState", "cnfsGroupId", "cnfsIplist").printHeader()
	newYRecord("mmlscluster", "cesSummary", "cesSharedRoot", "EnabledServices",
		"logLevel", "addressPolicy", "interfaceMode").printHeader()
	newYRecord("mmlscluster", "cesNode", "nodeNumber", "daemonNodeName", "ipAddress",
		"cesGroup", "cesState", "cesIpList").printHeader()
	newYRecord("mmlscluster", "cloudGatewayNode", "nodeNumber", "daemonNodeName").printHeader()
	commentRecord.printHeader()

	clusterRecord.printRow(summary.ClusterName, summary.ClusterID.String(), summary.UIDDomain,
		summary.RshPath, yesNo(summary.RshSudoWrapper), summary.RcpPath,
		yesNo(summary.RcpSudoWrapper), summary.RepositoryType,
		summary.PrimaryServer, summary.SecondaryServer)

	for _, node := range nodes {
		// mmlscluster reports the other roles twice: as an internal code the
		// REST API does not expose, and as the readable alias it does.
		nodeRecord.printRow(strconv.Itoa(node.NodeNumber), node.Network.DaemonNodeName,
			node.Network.DaemonIPAddress, node.AdminNodeName, node.Roles.Designation,
			"", adminLoginName(node), otherNodeRolesAlias(node))
	}
	for _, node := range nodes {
		commentRecord.printRow(strconv.Itoa(node.NodeNumber), node.Network.DaemonNodeName, "")
	}
}

// adminLoginName is reported only when the node is administered under a name
// other than root, which is what mmlscluster shows.
func adminLoginName(n nodeInfo) string {
	if n.Config.AdminLoginName == "root" {
		return ""
	}
	return n.Config.AdminLoginName
}

func otherNodeRolesAlias(n nodeInfo) string {
	var roles []string
	for _, role := range splitList(n.Roles.OtherNodeRoles) {
		roles = append(roles, strings.TrimSuffix(role, "Node"))
	}
	return strings.Join(roles, ",")
}

func columnWidth(header string, nodes []nodeInfo, field func(nodeInfo) string) int {
	width := len(header)
	for _, n := range nodes {
		if l := len(field(n)); l > width {
			width = l
		}
	}
	return width + 2
}

// nodeDesignation renders the GUI's role fields the way mmlscluster prints
// them: designation "quorumManager" plus otherNodeRoles "perfmonNode" becomes
// "quorum-manager-perfmon".
func nodeDesignation(n nodeInfo) string {
	parts := []string{}
	if d := camelToDashed(n.Roles.Designation); d != "" {
		parts = append(parts, d)
	}
	for _, role := range splitList(n.Roles.OtherNodeRoles) {
		role = strings.TrimSuffix(role, "Node")
		if d := camelToDashed(role); d != "" {
			parts = append(parts, d)
		}
	}
	return strings.Join(parts, "-")
}

func camelToDashed(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			if i > 0 {
				b.WriteByte('-')
			}
			c += 'a' - 'A'
		}
		b.WriteByte(c)
	}
	return b.String()
}
