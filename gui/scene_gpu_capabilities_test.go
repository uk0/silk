package gui

import "testing"

func TestSceneGPUCapabilitiesDoNotCreateContext(t *testing.T) {
	v := NewSceneView()
	defer v.Close()
	if v.GPUCapabilities().InstancedDrawing || v.gpu != nil || v.gpuAttempted {
		t.Fatal("capability query created a native backend")
	}
	v.SetRenderBackend("cpu")
	paintScene(t, v)
	if v.GPUCapabilities().InstancedDrawing || v.gpu != nil {
		t.Fatal("CPU renderer advertised instancing")
	}
	v.Close()
	if v.GPUCapabilities().InstancedDrawing {
		t.Fatal("closed view advertised GPU capability")
	}
}
