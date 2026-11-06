package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"opsagi/pkg/config"
)

func k8sTestTools(t *testing.T, paths *[]string, body string) *k8sTools {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if paths != nil {
			*paths = append(*paths, r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	return NewK8sTools(&config.Config{K8sAPIURL: server.URL})
}

func TestNewK8sTools_IsUnavailableWithoutAConfiguredCluster(t *testing.T) {
	if NewK8sTools(&config.Config{}).IsAvailable() {
		t.Fatal("a tool with nowhere to read must not be offered to an agent")
	}

	if !NewK8sTools(&config.Config{K8sAPIURL: "http://127.0.0.1:6444"}).IsAvailable() {
		t.Fatal("an API URL is enough to offer the tool")
	}
}

func TestK8s_HandleGet_SaysWhyAPodIsNotRunning(t *testing.T) {
	body := `{"items":[{"metadata":{"name":"web-5d8-abc","namespace":"shop"},
		"spec":{"nodeName":""},
		"status":{"phase":"Pending","containerStatuses":[{"name":"web","restartCount":3,
		"waiting":{"reason":"ImagePullBackOff","message":"Back-off pulling image \"nginx:1.99\""}}]}}]}`

	tools := k8sTestTools(t, nil, body)

	out, err := tools.HandleGet(context.Background(), K8sGetToolName, json.RawMessage(`{"what":"pods","namespace":"shop"}`))
	if err != nil {
		t.Fatalf("HandleGet: %v", err)
	}

	for _, want := range []string{"shop/web-5d8-abc", "Pending", "restarts=3", "ImagePullBackOff", "nginx:1.99"} {
		if !strings.Contains(out, want) {
			t.Errorf("the rendering is missing %q:\n%s", want, out)
		}
	}
}

func TestK8s_HandleGet_ReadsEventsWithTheirReasonAndCount(t *testing.T) {
	body := `{"items":[{"metadata":{"name":"web-5d8-abc.17f","namespace":"shop"},
		"involvedObject":{"kind":"Pod","name":"web-5d8-abc"},
		"reason":"FailedScheduling","message":"0/1 nodes are available: 1 node(s) had untolerated taint.",
		"count":12,"lastTimestamp":"2026-11-07T03:29:00Z"}]}`

	tools := k8sTestTools(t, nil, body)

	out, err := tools.HandleGet(context.Background(), K8sGetToolName, json.RawMessage(`{"what":"events"}`))
	if err != nil {
		t.Fatalf("HandleGet: %v", err)
	}

	for _, want := range []string{"Pod/web-5d8-abc", "FailedScheduling", "x12", "untolerated taint"} {
		if !strings.Contains(out, want) {
			t.Errorf("the rendering is missing %q:\n%s", want, out)
		}
	}
}

func TestK8s_HandleGet_AsksForANamespaceOnlyWhenOneIsGiven(t *testing.T) {
	tests := []struct {
		name string
		args string
		want string
	}{
		{"all namespaces", `{"what":"pods"}`, "/api/v1/pods"},
		{"one namespace", `{"what":"pods","namespace":"shop"}`, "/api/v1/namespaces/shop/pods"},
		{"deployments live under apps", `{"what":"deployments","namespace":"shop"}`, "/apis/apps/v1/namespaces/shop/deployments"},
		{"nodes are not namespaced", `{"what":"nodes","namespace":"shop"}`, "/api/v1/nodes"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var paths []string

			tools := k8sTestTools(t, &paths, `{"items":[]}`)
			if _, err := tools.HandleGet(context.Background(), K8sGetToolName, json.RawMessage(test.args)); err != nil {
				t.Fatalf("HandleGet: %v", err)
			}

			if len(paths) != 1 || paths[0] != test.want {
				t.Errorf("requested %v, want [%s]", paths, test.want)
			}
		})
	}
}

func TestK8s_HandleGet_ReportsAnEmptyAnswerAsAnEmptyAnswer(t *testing.T) {
	tools := k8sTestTools(t, nil, `{"items":[]}`)

	out, err := tools.HandleGet(context.Background(), K8sGetToolName, json.RawMessage(`{"what":"pods"}`))
	if err != nil {
		t.Fatalf("HandleGet: %v", err)
	}

	if !strings.Contains(out, "没有匹配的对象") {
		t.Errorf("an empty list has to read as empty, not as a finding:\n%s", out)
	}
}

func TestK8s_HandleGet_RefusesAKindItCannotRead(t *testing.T) {
	tools := k8sTestTools(t, nil, `{"items":[]}`)

	if _, err := tools.HandleGet(context.Background(), K8sGetToolName, json.RawMessage(`{"what":"secrets"}`)); err == nil {
		t.Fatal("a kind outside the enumeration must be refused, not fetched")
	}
}

func TestK8s_HandleGet_CarriesTheServerErrorIntoTheAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"pods is forbidden"}`))
	}))
	t.Cleanup(server.Close)

	tools := NewK8sTools(&config.Config{K8sAPIURL: server.URL})

	_, err := tools.HandleGet(context.Background(), K8sGetToolName, json.RawMessage(`{"what":"pods"}`))
	if err == nil || !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("the status and the reason have to survive, got %v", err)
	}
}

func TestParseKubeconfig_ReadsTheFirstClusterAndUser(t *testing.T) {
	ca := base64.StdEncoding.EncodeToString([]byte("ca-pem"))
	cert := base64.StdEncoding.EncodeToString([]byte("cert-pem"))
	key := base64.StdEncoding.EncodeToString([]byte("key-pem"))

	raw := []byte(`apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: ` + ca + `
    server: https://127.0.0.1:6443
  name: default
users:
- name: default
  user:
    client-certificate-data: ` + cert + `
    client-key-data: ` + key + `
current-context: default
`)

	kube, err := parseKubeconfig(raw, "C:/tmp/kubeconfig")
	if err != nil {
		t.Fatalf("parseKubeconfig: %v", err)
	}

	if kube.server != "https://127.0.0.1:6443" {
		t.Errorf("server = %q", kube.server)
	}

	if string(kube.ca) != "ca-pem" || string(kube.cert) != "cert-pem" || string(kube.key) != "key-pem" {
		t.Errorf("embedded material did not survive decoding: ca=%q cert=%q key=%q", kube.ca, kube.cert, kube.key)
	}

	if host := serverHost(kube.server); host != "127.0.0.1" {
		t.Errorf("serverHost = %q, want the host without the port", host)
	}
}

func TestParseKubeconfig_RefusesAFileWithoutACluster(t *testing.T) {
	if _, err := parseKubeconfig([]byte("apiVersion: v1\n"), "C:/tmp/kubeconfig"); err == nil {
		t.Fatal("a kubeconfig with no cluster must be an error, not an empty client")
	}
}
