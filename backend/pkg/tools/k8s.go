package tools

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"opsagi/pkg/config"
)

// K8sGetArgs reads one kind of object from a cluster. The kinds are the ones an
// incident is actually argued with: what is running, why it is not, and what the
// scheduler and the kubelet said about it.
type K8sGetArgs struct {
	What      string `json:"what" jsonschema:"required,enum=pods,enum=events,enum=deployments,enum=nodes" jsonschema_description:"要读哪一类对象：pods / events / deployments / nodes"`
	Namespace string `json:"namespace,omitempty" jsonschema:"description=命名空间，留空表示所有命名空间,example=shop"`
	Name      string `json:"name,omitempty" jsonschema:"description=只看名字里包含这段文字的对象,example=web-"`
}

const (
	k8sRequestTimeout = 30 * time.Second
	k8sMaxItems       = 200
)

type k8sTools struct {
	baseURL string
	client  *http.Client
	reason  string
}

// NewK8sTools builds the cluster reader from configuration. A cluster the tool
// cannot reach is worse than no tool at all: an agent would spend a call to learn
// that the configuration is missing, so the tool disappears from the list instead.
func NewK8sTools(cfg *config.Config) *k8sTools {
	if cfg == nil {
		return &k8sTools{reason: "not configured"}
	}

	client, baseURL, reason := k8sClientFromConfig(cfg)

	return &k8sTools{baseURL: baseURL, client: client, reason: reason}
}

func (k *k8sTools) IsAvailable() bool { return k.baseURL != "" }

// UnavailableReason explains a missing tool for the log line that drops it.
func (k *k8sTools) UnavailableReason() string { return k.reason }

func k8sClientFromConfig(cfg *config.Config) (*http.Client, string, string) {
	path := strings.TrimSpace(cfg.K8sKubeconfig)
	if path == "" && strings.TrimSpace(cfg.K8sAPIURL) == "" {
		if home, err := os.UserHomeDir(); err == nil {
			candidate := filepath.Join(home, ".kube", "config")
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
			}
		}
	}

	transport := &http.Transport{}

	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, "", fmt.Sprintf("cannot read %s: %v", path, err)
		}

		kube, err := parseKubeconfig(raw, path)
		if err != nil {
			return nil, "", err.Error()
		}

		tlsConfig, err := kube.tlsConfig(cfg)
		if err != nil {
			return nil, "", err.Error()
		}

		transport.TLSClientConfig = tlsConfig
		transport.TLSClientConfig.ServerName = serverHost(kube.server)

		return &http.Client{Timeout: k8sRequestTimeout, Transport: transport}, kube.server, ""
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}

	if cfg.K8sInsecureSkipTLSVerify {
		// A cluster reached over an SSH tunnel with a self-signed certificate is
		// the case this exists for; the operator opts in, the tool never does.
		tlsConfig.InsecureSkipVerify = true //nolint:gosec
	}

	transport.TLSClientConfig = tlsConfig

	return &http.Client{Timeout: k8sRequestTimeout, Transport: transport}, strings.TrimRight(cfg.K8sAPIURL, "/"), ""
}

type kubeCluster struct {
	server   string
	ca       []byte
	cert     []byte
	key      []byte
	token    string
	insecure bool
}

func (k kubeCluster) tlsConfig(cfg *config.Config) (*tls.Config, error) {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}

	if len(k.ca) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(k.ca) {
			return nil, fmt.Errorf("kubeconfig 里的 certificate-authority-data 不是 PEM 证书")
		}

		tlsConfig.RootCAs = pool
	}

	if len(k.cert) > 0 && len(k.key) > 0 {
		pair, err := tls.X509KeyPair(k.cert, k.key)
		if err != nil {
			return nil, fmt.Errorf("kubeconfig 的客户端证书无法加载：%w", err)
		}

		tlsConfig.Certificates = []tls.Certificate{pair}
	}

	if k.insecure || (cfg != nil && cfg.K8sInsecureSkipTLSVerify) {
		tlsConfig.InsecureSkipVerify = true //nolint:gosec
	}

	return tlsConfig, nil
}

// parseKubeconfig reads the first cluster and the first user of a kubeconfig,
// which is what every single-cluster setup has. Contexts are not followed: the
// tool is given one cluster, not a choice of them.
func parseKubeconfig(raw []byte, path string) (kubeCluster, error) {
	var doc struct {
		Clusters []struct {
			Cluster struct {
				Server                   string `yaml:"server"`
				CertificateAuthority     string `yaml:"certificate-authority"`
				CertificateAuthorityData string `yaml:"certificate-authority-data"`
				InsecureSkipTLSVerify    bool   `yaml:"insecure-skip-tls-verify"`
			} `yaml:"cluster"`
		} `yaml:"clusters"`
		Users []struct {
			User struct {
				Token                 string `yaml:"token"`
				ClientCertificate     string `yaml:"client-certificate"`
				ClientCertificateData string `yaml:"client-certificate-data"`
				ClientKey             string `yaml:"client-key"`
				ClientKeyData         string `yaml:"client-key-data"`
			} `yaml:"user"`
		} `yaml:"users"`
	}

	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return kubeCluster{}, fmt.Errorf("kubeconfig %s: %w", path, err)
	}

	if len(doc.Clusters) == 0 {
		return kubeCluster{}, fmt.Errorf("kubeconfig %s has no clusters", path)
	}

	cluster := doc.Clusters[0].Cluster
	if cluster.Server == "" {
		return kubeCluster{}, fmt.Errorf("kubeconfig %s has no server", path)
	}

	base := filepath.Dir(path)
	out := kubeCluster{
		server:   strings.TrimRight(cluster.Server, "/"),
		insecure: cluster.InsecureSkipTLSVerify,
	}

	var err error
	if out.ca, err = pemOrFile(cluster.CertificateAuthorityData, cluster.CertificateAuthority, base); err != nil {
		return kubeCluster{}, fmt.Errorf("kubeconfig %s: %w", path, err)
	}

	if len(doc.Users) > 0 {
		user := doc.Users[0].User
		out.token = user.Token
		if out.cert, err = pemOrFile(user.ClientCertificateData, user.ClientCertificate, base); err != nil {
			return kubeCluster{}, fmt.Errorf("kubeconfig %s: %w", path, err)
		}

		if out.key, err = pemOrFile(user.ClientKeyData, user.ClientKey, base); err != nil {
			return kubeCluster{}, fmt.Errorf("kubeconfig %s: %w", path, err)
		}
	}

	return out, nil
}

func pemOrFile(data, file, base string) ([]byte, error) {
	if data != "" {
		decoded, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, fmt.Errorf("embedder base64: %w", err)
		}

		return decoded, nil
	}

	if file == "" {
		return nil, nil
	}

	if !filepath.IsAbs(file) {
		file = filepath.Join(base, file)
	}

	return os.ReadFile(file)
}

func serverHost(server string) string {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(server, "https://"), "http://")
	if index := strings.IndexAny(trimmed, ":/"); index >= 0 {
		return trimmed[:index]
	}

	return trimmed
}

// HandleGet fetches one list from the cluster and renders what matters about it.
func (k *k8sTools) HandleGet(ctx context.Context, _ string, args json.RawMessage) (string, error) {
	if !k.IsAvailable() {
		return "", fmt.Errorf("没有配置集群：%s", k.reason)
	}

	var parsed K8sGetArgs
	if err := json.Unmarshal(args, &parsed); err != nil {
		return "", fmt.Errorf("解析参数失败：%w", err)
	}

	path, err := k8sPath(parsed)
	if err != nil {
		return "", err
	}

	body, err := k.get(ctx, path)
	if err != nil {
		return "", err
	}

	return renderK8s(parsed, body)
}

func k8sPath(args K8sGetArgs) (string, error) {
	var apiPrefix, resource string

	switch strings.TrimSpace(args.What) {
	case "pods":
		apiPrefix, resource = "/api/v1", "pods"
	case "events":
		apiPrefix, resource = "/api/v1", "events"
	case "deployments":
		apiPrefix, resource = "/apis/apps/v1", "deployments"
	case "nodes":
		return "/api/v1/nodes", nil
	default:
		return "", fmt.Errorf("what=%q 不是 pods / events / deployments / nodes 之一", args.What)
	}

	namespace := strings.TrimSpace(args.Namespace)
	if namespace == "" {
		return apiPrefix + "/" + resource, nil
	}

	return fmt.Sprintf("%s/namespaces/%s/%s", apiPrefix, namespace, resource), nil
}

func (k *k8sTools) get(ctx context.Context, path string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, k.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败：%w", err)
	}

	response, err := k.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("访问集群失败：%w", err)
	}

	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("读取集群响应失败：%w", err)
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("集群返回 %d：%s", response.StatusCode, firstLine(body))
	}

	return body, nil
}

func firstLine(body []byte) string {
	text := strings.TrimSpace(string(body))
	if index := strings.IndexByte(text, '\n'); index >= 0 {
		text = text[:index]
	}

	if len(text) > 300 {
		text = text[:300]
	}

	return text
}

type k8sList struct {
	Items []map[string]any `json:"items"`
}

func renderK8s(args K8sGetArgs, body []byte) (string, error) {
	var list k8sList
	if err := json.Unmarshal(body, &list); err != nil {
		return "", fmt.Errorf("集群响应不是列表：%w", err)
	}

	filter := strings.TrimSpace(args.Name)
	rows := make([]string, 0, len(list.Items))

	for _, item := range list.Items {
		name := k8sString(item, "metadata", "name")
		if filter != "" && !strings.Contains(name, filter) {
			continue
		}

		namespace := k8sString(item, "metadata", "namespace")
		if args.What == "events" {
			rows = append(rows, k8sEventRow(namespace, name, item))
		} else {
			rows = append(rows, k8sObjectRow(args.What, namespace, name, item))
		}
	}

	header := fmt.Sprintf("%s（%d 条，what=%s", k8sTitle(args.What), len(rows), args.What)
	if args.Namespace != "" {
		header += "，namespace=" + args.Namespace
	}

	header += "）"

	if len(rows) == 0 {
		return header + "\n没有匹配的对象：换一个 namespace 或去掉 name 过滤，再确认集群里确实有这类对象。", nil
	}

	sort.Strings(rows)

	if len(rows) > k8sMaxItems {
		rows = rows[:k8sMaxItems]
	}

	return header + "\n" + strings.Join(rows, "\n"), nil
}

func k8sTitle(what string) string {
	switch what {
	case "pods":
		return "Pod"
	case "events":
		return "事件"
	case "deployments":
		return "Deployment"
	case "nodes":
		return "节点"
	default:
		return what
	}
}

// k8sObjectRow keeps the columns an operator would read first: whether it runs,
// how often it has restarted, and — when it does not run — the sentence saying why.
func k8sObjectRow(what, namespace, name string, item map[string]any) string {
	prefix := namespace + "/" + name

	switch what {
	case "pods":
		phase := k8sString(item, "status", "phase")
		node := k8sString(item, "spec", "nodeName")
		restarts, reasons := k8sContainerStates(item)

		row := fmt.Sprintf("%-52s %-9s restarts=%d", prefix, phase, restarts)
		if node != "" {
			row += " node=" + node
		}

		if len(reasons) > 0 {
			row += " | " + strings.Join(reasons, "; ")
		}

		return row
	case "deployments":
		ready := k8sInt(item, "status", "readyReplicas")
		wanted := k8sInt(item, "spec", "replicas")

		return fmt.Sprintf("%-52s ready=%d/%d image=%s", prefix, ready, wanted, k8sDeploymentImages(item))
	case "nodes":
		ready := k8sNodeCondition(item)

		return fmt.Sprintf("%-52s %s", name, ready)
	}

	return prefix
}

// k8sEventRow answers "what did the control plane say about it, and when" — the
// reason and the message carry the diagnosis, the counters say whether it is
// still happening.
func k8sEventRow(namespace, name string, item map[string]any) string {
	object := k8sString(item, "involvedObject", "kind") + "/" + k8sString(item, "involvedObject", "name")
	reason := k8sString(item, "reason")
	message := k8sString(item, "message")
	count := k8sInt(item, "count")
	last := k8sString(item, "lastTimestamp")
	if last == "" {
		last = k8sString(item, "eventTime")
	}

	message = strings.Join(strings.Fields(message), " ")
	if len(message) > 200 {
		message = message[:200] + "…"
	}

	return fmt.Sprintf("%-20s %-8s %-22s x%-3d %-20s %s | %s",
		namespace, last, object, count, reason, name, message)
}

func k8sContainerStates(item map[string]any) (int, []string) {
	statuses, _ := dig(item, "status", "containerStatuses").([]any)

	restarts := 0
	reasons := make([]string, 0, len(statuses))

	for _, raw := range statuses {
		status, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		restarts += int(numOrZero(status["restartCount"]))

		for _, key := range []string{"waiting", "terminated"} {
			state, _ := status[key].(map[string]any)
			if state == nil {
				continue
			}

			reason := stringOrEmpty(state["reason"])
			if reason == "" {
				continue
			}

			entry := fmt.Sprintf("%s: %s", stringOrEmpty(status["name"]), reason)
			if message := stringOrEmpty(state["message"]); message != "" {
				if len(message) > 160 {
					message = message[:160] + "…"
				}

				entry += ": " + strings.Join(strings.Fields(message), " ")
			}

			reasons = append(reasons, entry)
		}
	}

	return restarts, reasons
}

func k8sDeploymentImages(item map[string]any) string {
	containers, _ := dig(item, "spec", "template", "spec", "containers").([]any)

	images := make([]string, 0, len(containers))

	for _, raw := range containers {
		container, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		if image := stringOrEmpty(container["image"]); image != "" {
			images = append(images, image)
		}
	}

	return strings.Join(images, ",")
}

func k8sNodeCondition(item map[string]any) string {
	conditions, _ := dig(item, "status", "conditions").([]any)

	for _, raw := range conditions {
		condition, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		if stringOrEmpty(condition["type"]) == "Ready" {
			return "Ready=" + stringOrEmpty(condition["status"])
		}
	}

	return "Ready=unknown"
}

func dig(item map[string]any, keys ...string) any {
	var current any = item

	for _, key := range keys {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil
		}

		current = mapping[key]
	}

	return current
}

func k8sString(item map[string]any, keys ...string) string {
	return stringOrEmpty(dig(item, keys...))
}

func k8sInt(item map[string]any, keys ...string) int {
	return int(numOrZero(dig(item, keys...)))
}

func stringOrEmpty(value any) string {
	text, _ := value.(string)

	return text
}

func numOrZero(value any) float64 {
	number, _ := value.(float64)

	return number
}
