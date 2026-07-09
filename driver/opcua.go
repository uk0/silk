package driver

import (
	"context"
	"errors"
	"fmt"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
)

// OPCUA is a Driver for OPC-UA servers over TCP using gopcua. Unlike Modbus and
// S7, OPC-UA is self-describing: each node carries its own typed value, so a
// point's Type and Order are not used to decode the wire bytes — the server
// hands back an already-typed ua.Variant. A point's Address is an OPC-UA NodeID
// in the standard textual form:
//
//	ns=2;s=Demo.Float   string identifier in namespace 2
//	ns=3;i=1001         numeric identifier in namespace 3
//	i=2258              numeric identifier in namespace 0
//
// parsed by ua.ParseNodeID. The read path returns the variant's Go value
// (float64, bool, int32, string, ...) and the write path wraps the value in a
// variant; the Poller normalizes numerics/bools to float64/bool.
type OPCUA struct {
	endpoint string // server endpoint, e.g. "opc.tcp://host:4840"

	client *opcua.Client
}

var _ Driver = (*OPCUA)(nil)

var errOPCUANotConnected = errors.New("driver: OPC-UA not connected")

// NewOPCUA builds a driver for the server at endpoint (form
// "opc.tcp://host:port").
func NewOPCUA(endpoint string) *OPCUA {
	return &OPCUA{endpoint: endpoint}
}

// Connect creates the client and opens the secure channel / session.
func (o *OPCUA) Connect() error {
	c, err := opcua.NewClient(o.endpoint)
	if err != nil {
		return err
	}
	if err := c.Connect(context.Background()); err != nil {
		return err
	}
	o.client = c
	return nil
}

// Close tears the session down. Safe to call when not connected.
func (o *OPCUA) Close() error {
	if o.client == nil {
		return nil
	}
	err := o.client.Close(context.Background())
	o.client = nil
	return err
}

// ReadPoint reads the Value attribute of the point's node and returns the
// variant's Go value. p.Order is ignored — the variant is already typed.
func (o *OPCUA) ReadPoint(p TagPoint) (interface{}, error) {
	if o.client == nil {
		return nil, errOPCUANotConnected
	}
	id, err := ua.ParseNodeID(p.Address)
	if err != nil {
		return nil, err
	}
	req := &ua.ReadRequest{
		NodesToRead: []*ua.ReadValueID{{NodeID: id, AttributeID: ua.AttributeIDValue}},
	}
	resp, err := o.client.Read(context.Background(), req)
	if err != nil {
		return nil, err
	}
	if len(resp.Results) == 0 {
		return nil, fmt.Errorf("driver: OPC-UA %s: empty read response", p.Address)
	}
	if s := resp.Results[0].Status; s != ua.StatusOK {
		return nil, s
	}
	return variantValue(resp.Results[0].Value), nil
}

// WritePoint wraps v in a variant and writes it to the Value attribute of the
// point's node. v is expected to be an OPC-UA builtin type (the Poller carries
// tag values as float64 or bool).
func (o *OPCUA) WritePoint(p TagPoint, v interface{}) error {
	if o.client == nil {
		return errOPCUANotConnected
	}
	id, err := ua.ParseNodeID(p.Address)
	if err != nil {
		return err
	}
	variant, err := ua.NewVariant(v)
	if err != nil {
		return err
	}
	req := &ua.WriteRequest{
		NodesToWrite: []*ua.WriteValue{{
			NodeID:      id,
			AttributeID: ua.AttributeIDValue,
			Value:       &ua.DataValue{EncodingMask: ua.DataValueValue, Value: variant},
		}},
	}
	resp, err := o.client.Write(context.Background(), req)
	if err != nil {
		return err
	}
	if len(resp.Results) == 0 {
		return fmt.Errorf("driver: OPC-UA %s: empty write response", p.Address)
	}
	if s := resp.Results[0]; s != ua.StatusOK {
		return s
	}
	return nil
}

// variantValue extracts the plain Go value an OPC-UA variant carries. A nil
// variant yields nil; otherwise the concrete type is whatever the server
// encoded (float64, bool, int32, string, ...).
func variantValue(v *ua.Variant) interface{} {
	if v == nil {
		return nil
	}
	return v.Value()
}
