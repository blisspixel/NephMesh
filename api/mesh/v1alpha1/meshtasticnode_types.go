/*
Copyright 2026 The NephMesh Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MeshtasticNodeSpec is the desired state of one Meshtastic node.
//
// Fields mirror the real device configuration surface reachable over the
// TCP 4403 client API. Enum-like fields (region, modemPreset, role) are open
// strings with a shape pattern rather than closed enums, so a firmware update
// that adds a value does not reject the API until the CRD is re-released; the
// controller keeps known values as constants and degrades gracefully on
// anything it does not recognize.
type MeshtasticNodeSpec struct {
	// connection selects exactly one transport to the device.
	Connection ConnectionSpec `json:"connection"`

	// region maps to lora.region (for example US, EU_868). Required.
	// +kubebuilder:validation:Pattern=`^[A-Z0-9_]+$`
	// +kubebuilder:validation:MaxLength=32
	Region string `json:"region"`

	// modemPreset maps to lora.modem_preset (for example LONG_FAST, MEDIUM_SLOW).
	// +kubebuilder:validation:Pattern=`^[A-Z0-9_]+$`
	// +kubebuilder:validation:MaxLength=32
	// +optional
	ModemPreset string `json:"modemPreset,omitempty"`

	// role maps to device.role (for example CLIENT, ROUTER, REPEATER).
	// +kubebuilder:validation:Pattern=`^[A-Z0-9_]+$`
	// +kubebuilder:validation:MaxLength=32
	// +optional
	Role string `json:"role,omitempty"`

	// owner sets the node's short and long names.
	// +optional
	Owner *OwnerSpec `json:"owner,omitempty"`

	// channels is the desired channel set, by index. PSKs are referenced from
	// Secrets, never inlined.
	// +optional
	// +listType=map
	// +listMapKey=index
	Channels []ChannelSpec `json:"channels,omitempty"`

	// mqtt configures the device MQTT module.
	// +optional
	MQTT *MQTTSpec `json:"mqtt,omitempty"`

	// deletionPolicy decides what happens to the physical radio when this
	// resource is deleted. Retain (default) stops managing it and leaves it
	// running with its last-applied config. Wipe factory-resets it during
	// finalization.
	// +kubebuilder:validation:Enum=Retain;Wipe
	// +kubebuilder:default=Retain
	// +optional
	DeletionPolicy string `json:"deletionPolicy,omitempty"`
}

// ConnectionSpec selects exactly one transport to the device. The CEL rule
// enforces the mutual exclusion in the API server, so the operator ships
// without a validating webhook.
//
// +kubebuilder:validation:XValidation:rule="[has(self.tcp), has(self.serial), has(self.viaGateway)].filter(x, x).size() == 1",message="exactly one of tcp, serial, or viaGateway must be set"
type ConnectionSpec struct {
	// tcp reaches the device over its TCP client API (port 4403 by default).
	// +optional
	TCP *TCPConnection `json:"tcp,omitempty"`

	// serial reaches the device over a serial port exposed to the pod.
	// +optional
	Serial *SerialConnection `json:"serial,omitempty"`

	// viaGateway manages a radio-only node remotely through a managed gateway
	// node, using the admin channel.
	// +optional
	ViaGateway *ViaGatewayConnection `json:"viaGateway,omitempty"`
}

// TCPConnection reaches the device over TCP.
type TCPConnection struct {
	// host is the device address (a Service name, FQDN, or IP). The pattern
	// forbids a leading dash so the value cannot be misread as a CLI flag, and
	// bounds the length; it is a hostname or IP shape, not an arbitrary string,
	// to limit where the operator can be pointed.
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?$`
	// +kubebuilder:validation:MaxLength=253
	Host string `json:"host"`
	// port defaults to 4403.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:default=4403
	// +optional
	Port int32 `json:"port,omitempty"`
}

// SerialConnection reaches the device over a serial port.
type SerialConnection struct {
	// device is the serial device path, for example /dev/ttyUSB0.
	Device string `json:"device"`
}

// ViaGatewayConnection manages a remote radio-only node through a gateway.
type ViaGatewayConnection struct {
	// gatewayRef names another MeshtasticNode that carries the admin channel.
	GatewayRef corev1.LocalObjectReference `json:"gatewayRef"`
	// dest is the target radio node id, for example "!6e000001".
	Dest string `json:"dest"`
}

// OwnerSpec sets the device owner names.
type OwnerSpec struct {
	// shortName is limited to a few characters on the device.
	// +kubebuilder:validation:MaxLength=4
	// +optional
	ShortName string `json:"shortName,omitempty"`
	// longName is the full node name.
	// +kubebuilder:validation:MaxLength=40
	// +optional
	LongName string `json:"longName,omitempty"`
}

// ChannelSpec is one Meshtastic channel.
type ChannelSpec struct {
	// index is the channel slot; 0 is the primary channel.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=7
	Index int32 `json:"index"`
	// name is the channel name.
	// +optional
	Name string `json:"name,omitempty"`
	// pskSecretRef references the channel pre-shared key. It is never inlined.
	// Omitting it uses the device default, which is discouraged because the
	// default PSK is public.
	// +optional
	PSKSecretRef *corev1.SecretKeySelector `json:"pskSecretRef,omitempty"`
	// uplinkEnabled forwards this channel's traffic to MQTT.
	// +optional
	UplinkEnabled bool `json:"uplinkEnabled,omitempty"`
	// downlinkEnabled injects MQTT traffic onto this channel.
	// +optional
	DownlinkEnabled bool `json:"downlinkEnabled,omitempty"`
}

// MQTTSpec mirrors the device MQTT module configuration.
type MQTTSpec struct {
	// enabled turns the MQTT module on.
	Enabled bool `json:"enabled"`
	// address is the broker host.
	// +optional
	Address string `json:"address,omitempty"`
	// username is the broker username.
	// +optional
	Username string `json:"username,omitempty"`
	// passwordSecretRef references the broker password. It is never inlined.
	// +optional
	PasswordSecretRef *corev1.SecretKeySelector `json:"passwordSecretRef,omitempty"`
	// encryptionEnabled keeps payloads channel-encrypted over MQTT.
	// +optional
	EncryptionEnabled bool `json:"encryptionEnabled,omitempty"`
	// jsonEnabled publishes lossy JSON topics (unsupported on nRF52 devices).
	// +optional
	JSONEnabled bool `json:"jsonEnabled,omitempty"`
	// tlsEnabled uses TLS to the broker.
	// +optional
	TLSEnabled bool `json:"tlsEnabled,omitempty"`
	// root overrides the MQTT root topic.
	// +optional
	Root string `json:"root,omitempty"`
}

// MeshtasticNodeStatus is the observed state of a Meshtastic node.
type MeshtasticNodeStatus struct {
	// conditions follow the standard Kubernetes convention. Reachability and
	// sync state live here rather than in duplicate boolean fields.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// observedGeneration is the generation last reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// nodeID is the device node id, for example "!6e000001".
	// +optional
	NodeID string `json:"nodeID,omitempty"`
	// lastHeard is when the device was last reachable.
	// +optional
	LastHeard *metav1.Time `json:"lastHeard,omitempty"`
	// applyAttempts counts consecutive config applies that have not yet
	// converged. It resets to zero on convergence and bounds the reboot loop:
	// after too many attempts the node is marked Degraded rather than rebooting
	// forever, which protects against a desired field the device never echoes.
	// +optional
	ApplyAttempts int32 `json:"applyAttempts,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=nephmesh
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Region",type=string,JSONPath=`.spec.region`
// +kubebuilder:printcolumn:name="NodeID",type=string,JSONPath=`.status.nodeID`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// MeshtasticNode is the declared desired state of one Meshtastic node.
type MeshtasticNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MeshtasticNodeSpec   `json:"spec,omitempty"`
	Status MeshtasticNodeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MeshtasticNodeList is a list of MeshtasticNode.
type MeshtasticNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MeshtasticNode `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MeshtasticNode{}, &MeshtasticNodeList{})
}
