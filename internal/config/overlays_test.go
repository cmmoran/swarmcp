package config

import (
	"strings"
	"testing"
)

func TestStackServicesFiltersIncludedInByTarget(t *testing.T) {
	cfg := &Config{
		Project: Project{
			Name:        "primary",
			Deployment:  "prod",
			Deployments: []string{"prod"},
			Partitions:  []string{"blue", "green"},
		},
		Stacks: map[string]Stack{
			"participant": {
				Mode: "partitioned",
				Services: map[string]Service{
					"api": {
						Image: "api",
						IncludedIn: []ServiceInclusionRule{
							{
								Deployments: []string{"prod"},
								Partitions:  []string{"blue"},
								Stacks:      []string{"participant"},
							},
						},
					},
					"worker": {Image: "worker"},
				},
			},
		},
	}

	blueServices, err := cfg.StackServices("participant", "blue")
	if err != nil {
		t.Fatalf("blue services: %v", err)
	}
	if _, ok := blueServices["api"]; !ok {
		t.Fatalf("expected api in blue target, got %#v", blueServices)
	}
	if _, ok := blueServices["worker"]; !ok {
		t.Fatalf("expected worker in blue target, got %#v", blueServices)
	}

	greenServices, err := cfg.StackServices("participant", "green")
	if err != nil {
		t.Fatalf("green services: %v", err)
	}
	if _, ok := greenServices["api"]; ok {
		t.Fatalf("expected api to be excluded in green target, got %#v", greenServices)
	}
	if _, ok := greenServices["worker"]; !ok {
		t.Fatalf("expected worker in green target, got %#v", greenServices)
	}
}

func TestStackServicesIgnoresPartitionRulesForSharedStacks(t *testing.T) {
	cfg := &Config{
		Project: Project{
			Name:        "primary",
			Deployment:  "prod",
			Deployments: []string{"prod"},
			Partitions:  []string{"blue", "green"},
		},
		Stacks: map[string]Stack{
			"primary-ext": {
				Mode: "shared",
				Services: map[string]Service{
					"grafana": {
						Image: "grafana",
						IncludedIn: []ServiceInclusionRule{
							{
								Deployments: []string{"prod"},
								Partitions:  []string{"blue"},
							},
						},
					},
				},
			},
		},
	}

	services, err := cfg.StackServices("primary-ext", "")
	if err != nil {
		t.Fatalf("stack services: %v", err)
	}
	if _, ok := services["grafana"]; !ok {
		t.Fatalf("expected grafana to remain included for shared stack, got %#v", services)
	}
}

func TestStackServicesReturnsEmptyWhenStackExcluded(t *testing.T) {
	cfg := &Config{
		Project: Project{
			Name:        "primary",
			Deployment:  "prod",
			Deployments: []string{"prod", "nonprod"},
			Partitions:  []string{"blue"},
		},
		Stacks: map[string]Stack{
			"primary-ext": {
				Mode: "shared",
				IncludedIn: []InclusionRule{
					{Deployments: []string{"nonprod"}},
				},
				Services: map[string]Service{
					"grafana": {Image: "grafana"},
				},
			},
		},
	}

	services, err := cfg.StackServices("primary-ext", "")
	if err != nil {
		t.Fatalf("stack services: %v", err)
	}
	if len(services) != 0 {
		t.Fatalf("expected excluded stack to have no services, got %#v", services)
	}
}

func TestStackServicesFiltersIncludedInFromServiceOverlay(t *testing.T) {
	cfg := &Config{
		Project: Project{
			Name:        "primary",
			Deployment:  "prod",
			Deployments: []string{"prod"},
			Partitions:  []string{"blue", "green"},
		},
		Stacks: map[string]Stack{
			"participant": {
				Mode: "partitioned",
				Services: map[string]Service{
					"api": {
						Image: "api",
						Overlays: ServiceOverlays{
							Deployments: map[string]OverlayService{
								"prod": {
									Fields: map[string]any{
										"included_in": []any{
											map[string]any{"partitions": []any{"blue"}},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	blueServices, err := cfg.StackServices("participant", "blue")
	if err != nil {
		t.Fatalf("blue services: %v", err)
	}
	if _, ok := blueServices["api"]; !ok {
		t.Fatalf("expected api in blue target, got %#v", blueServices)
	}

	greenServices, err := cfg.StackServices("participant", "green")
	if err != nil {
		t.Fatalf("green services: %v", err)
	}
	if _, ok := greenServices["api"]; ok {
		t.Fatalf("expected api to be excluded in green target, got %#v", greenServices)
	}
}

func TestStackServicesRejectsDependencyOnExcludedService(t *testing.T) {
	cfg := &Config{
		Project: Project{
			Name:        "primary",
			Deployment:  "prod",
			Deployments: []string{"prod"},
			Partitions:  []string{"blue"},
		},
		Stacks: map[string]Stack{
			"participant": {
				Mode: "partitioned",
				Services: map[string]Service{
					"api": {
						Image:     "api",
						DependsOn: []string{"db"},
					},
					"db": {
						Image: "db",
						IncludedIn: []ServiceInclusionRule{
							{Deployments: []string{"dev"}},
						},
					},
				},
			},
		},
	}

	_, err := cfg.StackServices("participant", "blue")
	if err == nil {
		t.Fatalf("expected dependency validation error")
	}
	if got := err.Error(); got == "" || !strings.Contains(got, `service "db" is not included for this target`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSealedOverlayPrecedence(t *testing.T) {
	cfg := &Config{
		Project: Project{
			Name:       "primary",
			Deployment: "prod",
			Partitions: []string{"p1"},
		},
		Stacks: map[string]Stack{
			"core": {
				Services: map[string]Service{
					"api": {Image: "base"},
				},
			},
		},
		Overlays: Overlays{
			Deployments: map[string]Overlay{
				"prod": {
					Stacks: map[string]OverlayStack{
						"core": {
							Sealed: true,
							Services: map[string]OverlayService{
								"api": {
									Fields: map[string]any{
										"image": "deploy",
									},
								},
							},
						},
					},
				},
			},
			Partitions: PartitionOverlays{
				Rules: []PartitionOverlay{
					{
						Name:  "p1",
						Match: OverlayMatch{Partition: OverlayMatchPartition{Pattern: "p1"}},
						Overlay: Overlay{
							Stacks: map[string]OverlayStack{
								"core": {
									Services: map[string]OverlayService{
										"api": {
											Sealed: true,
											Fields: map[string]any{
												"image": "partition",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	services, err := cfg.StackServices("core", "p1")
	if err != nil {
		t.Fatalf("stack services: %v", err)
	}
	service, ok := services["api"]
	if !ok {
		t.Fatalf("expected api service")
	}
	if service.Image != "deploy" {
		t.Fatalf("expected sealed deployment overlay to win; got %q", service.Image)
	}
}

func TestOverlayPrecedenceUnsealed(t *testing.T) {
	cfg := &Config{
		Project: Project{
			Name:       "primary",
			Deployment: "prod",
			Partitions: []string{"p1"},
		},
		Stacks: map[string]Stack{
			"core": {
				Services: map[string]Service{
					"api": {
						Image: "base",
						Overlays: ServiceOverlays{
							Deployments: map[string]OverlayService{
								"prod": {
									Fields: map[string]any{"image": "service-deploy"},
								},
							},
							Partitions: ServicePartitionOverlays{
								Rules: []ServicePartitionOverlay{
									{
										Name:  "p1",
										Match: OverlayMatch{Partition: OverlayMatchPartition{Pattern: "p1"}},
										Service: OverlayService{
											Fields: map[string]any{"image": "service-partition"},
										},
									},
								},
							},
						},
					},
				},
				Overlays: StackOverlays{
					Deployments: map[string]OverlayStack{
						"prod": {
							Services: map[string]OverlayService{
								"api": {Fields: map[string]any{"image": "stack-deploy"}},
							},
						},
					},
					Partitions: StackPartitionOverlays{
						Rules: []StackPartitionOverlay{
							{
								Name:  "p1",
								Match: OverlayMatch{Partition: OverlayMatchPartition{Pattern: "p1"}},
								OverlayStack: OverlayStack{
									Services: map[string]OverlayService{
										"api": {Fields: map[string]any{"image": "stack-partition"}},
									},
								},
							},
						},
					},
				},
			},
		},
		Overlays: Overlays{
			Deployments: map[string]Overlay{
				"prod": {
					Stacks: map[string]OverlayStack{
						"core": {
							Services: map[string]OverlayService{
								"api": {Fields: map[string]any{"image": "project-deploy"}},
							},
						},
					},
				},
			},
			Partitions: PartitionOverlays{
				Rules: []PartitionOverlay{
					{
						Name:  "p1",
						Match: OverlayMatch{Partition: OverlayMatchPartition{Pattern: "p1"}},
						Overlay: Overlay{
							Stacks: map[string]OverlayStack{
								"core": {
									Services: map[string]OverlayService{
										"api": {Fields: map[string]any{"image": "project-partition"}},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	services, err := cfg.StackServices("core", "p1")
	if err != nil {
		t.Fatalf("stack services: %v", err)
	}
	service, ok := services["api"]
	if !ok {
		t.Fatalf("expected api service")
	}
	if service.Image != "service-partition" {
		t.Fatalf("expected service partition overlay to win; got %q", service.Image)
	}
}

func TestOverlayPrecedenceSealedProjectWins(t *testing.T) {
	cfg := &Config{
		Project: Project{
			Name:       "primary",
			Deployment: "prod",
			Partitions: []string{"p1"},
		},
		Stacks: map[string]Stack{
			"core": {
				Services: map[string]Service{
					"api": {
						Image: "base",
						Overlays: ServiceOverlays{
							Deployments: map[string]OverlayService{
								"prod": {
									Sealed: true,
									Fields: map[string]any{"image": "service-sealed"},
								},
							},
						},
					},
				},
				Overlays: StackOverlays{
					Deployments: map[string]OverlayStack{
						"prod": {
							Sealed: true,
							Services: map[string]OverlayService{
								"api": {Fields: map[string]any{"image": "stack-sealed"}},
							},
						},
					},
				},
			},
		},
		Overlays: Overlays{
			Deployments: map[string]Overlay{
				"prod": {
					Stacks: map[string]OverlayStack{
						"core": {
							Sealed: true,
							Services: map[string]OverlayService{
								"api": {Fields: map[string]any{"image": "project-sealed"}},
							},
						},
					},
				},
			},
		},
	}

	services, err := cfg.StackServices("core", "p1")
	if err != nil {
		t.Fatalf("stack services: %v", err)
	}
	service, ok := services["api"]
	if !ok {
		t.Fatalf("expected api service")
	}
	if service.Image != "project-sealed" {
		t.Fatalf("expected project sealed overlay to win; got %q", service.Image)
	}
}

func TestOverlayPrecedenceSealedStackWinsOverService(t *testing.T) {
	cfg := &Config{
		Project: Project{
			Name:       "primary",
			Deployment: "prod",
			Partitions: []string{"p1"},
		},
		Stacks: map[string]Stack{
			"core": {
				Services: map[string]Service{
					"api": {
						Image: "base",
						Overlays: ServiceOverlays{
							Partitions: ServicePartitionOverlays{
								Rules: []ServicePartitionOverlay{
									{
										Name:  "p1",
										Match: OverlayMatch{Partition: OverlayMatchPartition{Pattern: "p1"}},
										Service: OverlayService{
											Sealed: true,
											Fields: map[string]any{"image": "service-sealed"},
										},
									},
								},
							},
						},
					},
				},
				Overlays: StackOverlays{
					Partitions: StackPartitionOverlays{
						Rules: []StackPartitionOverlay{
							{
								Name:  "p1",
								Match: OverlayMatch{Partition: OverlayMatchPartition{Pattern: "p1"}},
								OverlayStack: OverlayStack{
									Sealed: true,
									Services: map[string]OverlayService{
										"api": {Fields: map[string]any{"image": "stack-sealed"}},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	services, err := cfg.StackServices("core", "p1")
	if err != nil {
		t.Fatalf("stack services: %v", err)
	}
	service, ok := services["api"]
	if !ok {
		t.Fatalf("expected api service")
	}
	if service.Image != "stack-sealed" {
		t.Fatalf("expected stack sealed overlay to win; got %q", service.Image)
	}
}
