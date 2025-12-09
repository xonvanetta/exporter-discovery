package k8s

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	v1alpha1client "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned/typed/monitoring/v1alpha1"
	"github.com/xonvanetta/exporter-discovery/internal/scanner"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
)

const ExporterExporterPort = "9999"

type Module struct {
	Name     string
	Interval string
	Timeout  string
}

type Client struct {
	client    v1alpha1client.MonitoringV1alpha1Interface
	namespace string
	podRef    *metav1.OwnerReference
	Modules   []Module
}

func NewClient(namespace string, moduleConfigs []Module) (*Client, error) {
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	)

	restConfig, err := kubeconfig.ClientConfig()
	if err != nil {
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to get kubernetes config: %w", err)
		}
	}

	monClient, err := v1alpha1client.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create monitoring client: %w", err)
	}

	client := &Client{
		client:    monClient,
		namespace: namespace,
		Modules:   moduleConfigs,
	}

	podName := os.Getenv("POD_NAME")
	podUID := os.Getenv("POD_UID")

	if podName != "" && podUID != "" {
		client.podRef = &metav1.OwnerReference{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Pod",
			Name:       podName,
			UID:        types.UID(podUID),
		}
	}

	return client, nil
}

func (c *Client) getModuleConfig(name string) *Module {
	for _, m := range c.Modules {
		if m.Name == name {
			return &m
		}
	}
	return nil
}

func (c *Client) UpdateScrapeConfigs(ctx context.Context, targets map[string][]scanner.Target) error {
	for moduleName, moduleTargets := range targets {
		if err := c.updateScrapeConfig(ctx, moduleName, moduleTargets); err != nil {
			return fmt.Errorf("failed to update scrapeconfig for module %s: %w", moduleName, err)
		}
	}
	return nil
}

func (c *Client) buildStaticConfigs(moduleName string, targets []scanner.Target) []monitoringv1alpha1.StaticConfig {
	staticConfigs := make([]monitoringv1alpha1.StaticConfig, 0, len(targets))
	for _, target := range targets {
		staticConfigs = append(staticConfigs, monitoringv1alpha1.StaticConfig{
			Targets: []monitoringv1alpha1.Target{
				monitoringv1alpha1.Target(net.JoinHostPort(target.IP, ExporterExporterPort)),
			},
			Labels: map[monitoringv1.LabelName]string{
				"host":             target.Hostname,
				"module":           moduleName,
				"__metrics_path__": "/proxy",
				"__param_module":   moduleName,
			},
		})
	}
	return staticConfigs
}

func (c *Client) applyModuleConfig(sc *monitoringv1alpha1.ScrapeConfig, moduleName string) {
	if c.podRef != nil {
		sc.OwnerReferences = []metav1.OwnerReference{*c.podRef}
	}

	if module := c.getModuleConfig(moduleName); module != nil {
		if module.Interval != "" {
			sc.Spec.ScrapeInterval = ptr.To(monitoringv1.Duration(module.Interval))
		}
		if module.Timeout != "" {
			sc.Spec.ScrapeTimeout = ptr.To(monitoringv1.Duration(module.Timeout))
		}
	}
}

func (c *Client) mergeStaticConfigs(existing, new []monitoringv1alpha1.StaticConfig) []monitoringv1alpha1.StaticConfig {
	newTargets := make(map[string]monitoringv1alpha1.StaticConfig)
	for _, config := range new {
		if len(config.Targets) > 0 {
			target := string(config.Targets[0])
			newTargets[target] = config
		}
	}

	existingTargets := make(map[string]monitoringv1alpha1.StaticConfig)
	for _, config := range existing {
		if len(config.Targets) > 0 {
			target := string(config.Targets[0])
			existingTargets[target] = config
		}
	}

	preserved := 0
	for target, config := range existingTargets {
		if _, found := newTargets[target]; !found {
			newTargets[target] = config
			preserved++
			hostname := config.Labels["host"]
			log.Printf("Preserving target %s (%s) - not found in current scan", target, hostname)
		}
	}

	result := make([]monitoringv1alpha1.StaticConfig, 0, len(newTargets))
	for _, config := range newTargets {
		result = append(result, config)
	}

	return result
}

func (c *Client) updateScrapeConfig(ctx context.Context, moduleName string, targets []scanner.Target) error {
	scrapeConfigName := fmt.Sprintf("exporter-discovery-%s", moduleName)
	newStaticConfigs := c.buildStaticConfigs(moduleName, targets)

	sc, err := c.client.ScrapeConfigs(c.namespace).Get(ctx, scrapeConfigName, metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if errors.IsNotFound(err) {
		sc = &monitoringv1alpha1.ScrapeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: scrapeConfigName,
			},
			Spec: monitoringv1alpha1.ScrapeConfigSpec{
				StaticConfigs: c.buildStaticConfigs(moduleName, targets),
			},
		}
		c.applyModuleConfig(sc, moduleName)

		_, err = c.client.ScrapeConfigs(c.namespace).Create(ctx, sc, metav1.CreateOptions{})
		return err
	}

	sc.Spec.StaticConfigs = c.mergeStaticConfigs(sc.Spec.StaticConfigs, newStaticConfigs)
	c.applyModuleConfig(sc, moduleName)

	_, err = c.client.ScrapeConfigs(c.namespace).Update(ctx, sc, metav1.UpdateOptions{})
	return err
}
