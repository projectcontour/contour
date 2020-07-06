// Copyright Project Contour Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package envoy

import (
	"path"
	"testing"

	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_bootstrap "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v2"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/projectcontour/contour/internal/assert"
)

func TestBootstrap(t *testing.T) {
	tests := map[string]struct {
		config                        BootstrapConfig
		wantedBootstrapConfig         string
		wantedTLSCertificateConfig    string
		wantedValidationContextConfig string
		wantedError                   bool
	}{
		"default configuration": {
			config: BootstrapConfig{
				Path:      "envoy.json",
				Namespace: "testing-ns"},
			wantedBootstrapConfig: `{
  "static_resources": {
    "clusters": [
      {
        "name": "contour",
        "alt_stat_name": "testing-ns_contour_8001",
        "type": "STRICT_DNS",
        "connect_timeout": "5s",
        "load_assignment": {
          "cluster_name": "contour",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "127.0.0.1",
                        "port_value": 8001
                      }
                    }
                  }
                }
              ]
            }
          ]
        },
        "circuit_breakers": {
          "thresholds": [
            {
              "priority": "HIGH",
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50
            },
            {
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50
            }
          ]
        },
        "http2_protocol_options": {},
        "upstream_connection_options": {
          "tcp_keepalive": {
            "keepalive_probes": 3,
            "keepalive_time": 30,
            "keepalive_interval": 5
          }
        }
      },
      {
        "name": "service-stats",
        "alt_stat_name": "testing-ns_service-stats_9001",
        "type": "LOGICAL_DNS",
        "connect_timeout": "0.250s",
        "load_assignment": {
          "cluster_name": "service-stats",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "127.0.0.1",
                        "port_value": 9001
                      }
                    }
                  }
                }
              ]
            }
          ]
        }
      }
    ]
  },
  "dynamic_resources": {
    "lds_config": {
      "api_config_source": {
        "api_type": "GRPC",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour"
            }
          }
        ]
      }
    },
    "cds_config": {
      "api_config_source": {
        "api_type": "GRPC",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour"
            }
          }
        ]
      }
    }
  },
  "admin": {
    "access_log_path": "/dev/null",
    "address": {
      "socket_address": {
        "address": "127.0.0.1",
        "port_value": 9001
      }
    }
  }
}`,
		},
		"--admin-address=8.8.8.8 --admin-port=9200": {
			config: BootstrapConfig{
				Path:         "envoy.json",
				AdminAddress: "8.8.8.8",
				AdminPort:    9200,
				Namespace:    "testing-ns",
			},
			wantedBootstrapConfig: `{
  "static_resources": {
    "clusters": [
      {
        "name": "contour",
        "alt_stat_name": "testing-ns_contour_8001",
        "type": "STRICT_DNS",
        "connect_timeout": "5s",
        "load_assignment": {
          "cluster_name": "contour",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "127.0.0.1",
                        "port_value": 8001
                      }
                    }
                  }
                }
              ]
            }
          ]
        },
        "circuit_breakers": {
          "thresholds": [
            {
              "priority": "HIGH",
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50
            },
            {
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50
            }
          ]
        },
        "http2_protocol_options": {},
        "upstream_connection_options": {
          "tcp_keepalive": {
            "keepalive_probes": 3,
            "keepalive_time": 30,
            "keepalive_interval": 5
          }
        }
      },
      {
        "name": "service-stats",
        "alt_stat_name": "testing-ns_service-stats_9200",
        "type": "LOGICAL_DNS",
        "connect_timeout": "0.250s",
        "load_assignment": {
          "cluster_name": "service-stats",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "8.8.8.8",
                        "port_value": 9200
                      }
                    }
                  }
                }
              ]
            }
          ]
        }
      }
    ]
  },
  "dynamic_resources": {
    "lds_config": {
      "api_config_source": {
        "api_type": "GRPC",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour"
            }
          }
        ]
      }
    },
    "cds_config": {
      "api_config_source": {
        "api_type": "GRPC",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour"
            }
          }
        ]
      }
    }
  },
  "admin": {
    "access_log_path": "/dev/null",
    "address": {
      "socket_address": {
        "address": "8.8.8.8",
        "port_value": 9200
      }
    }
  }
}`,
		},
		"AdminAccessLogPath": { // TODO(dfc) doesn't appear to be exposed via contour bootstrap
			config: BootstrapConfig{
				Path:               "envoy.json",
				AdminAccessLogPath: "/var/log/admin.log",
				Namespace:          "testing-ns",
			},
			wantedBootstrapConfig: `{
  "static_resources": {
    "clusters": [
      {
        "name": "contour",
        "alt_stat_name": "testing-ns_contour_8001",
        "type": "STRICT_DNS",
        "connect_timeout": "5s",
        "load_assignment": {
          "cluster_name": "contour",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "127.0.0.1",
                        "port_value": 8001
                      }
                    }
                  }
                }
              ]
            }
          ]
        },
        "circuit_breakers": {
          "thresholds": [
            {
              "priority": "HIGH",
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50
            },
            {
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50
            }
          ]
        },
        "http2_protocol_options": {},
        "upstream_connection_options": {
          "tcp_keepalive": {
            "keepalive_probes": 3,
            "keepalive_time": 30,
            "keepalive_interval": 5
          }
        }
      },
      {
        "name": "service-stats",
        "alt_stat_name": "testing-ns_service-stats_9001",
        "type": "LOGICAL_DNS",
        "connect_timeout": "0.250s",
        "load_assignment": {
          "cluster_name": "service-stats",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "127.0.0.1",
                        "port_value": 9001
                      }
                    }
                  }
                }
              ]
            }
          ]
        }
      }
    ]
  },
  "dynamic_resources": {
    "lds_config": {
      "api_config_source": {
        "api_type": "GRPC",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour"
            }
          }
        ]
      }
    },
    "cds_config": {
      "api_config_source": {
        "api_type": "GRPC",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour"
            }
          }
        ]
      }
    }
  },
  "admin": {
    "access_log_path": "/var/log/admin.log",
    "address": {
      "socket_address": {
        "address": "127.0.0.1",
        "port_value": 9001
      }
    }
  }
}`,
		},
		"--xds-address=8.8.8.8 --xds-port=9200": {
			config: BootstrapConfig{
				Path:        "envoy.json",
				XDSAddress:  "8.8.8.8",
				XDSGRPCPort: 9200,
				Namespace:   "testing-ns",
			},
			wantedBootstrapConfig: `{
  "static_resources": {
    "clusters": [
      {
        "name": "contour",
        "alt_stat_name": "testing-ns_contour_9200",
        "type": "STRICT_DNS",
        "connect_timeout": "5s",
        "load_assignment": {
          "cluster_name": "contour",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "8.8.8.8",
                        "port_value": 9200
                      }
                    }
                  }
                }
              ]
            }
          ]
        },
        "circuit_breakers": {
          "thresholds": [
            {
              "priority": "HIGH",
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50
            },
            {
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50
            }
          ]
        },
        "http2_protocol_options": {},
        "upstream_connection_options": {
          "tcp_keepalive": {
            "keepalive_probes": 3,
            "keepalive_time": 30,
            "keepalive_interval": 5
          }
        }
      },
      {
        "name": "service-stats",
        "alt_stat_name": "testing-ns_service-stats_9001",
        "type": "LOGICAL_DNS",
        "connect_timeout": "0.250s",
        "load_assignment": {
          "cluster_name": "service-stats",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "127.0.0.1",
                        "port_value": 9001
                      }
                    }
                  }
                }
              ]
            }
          ]
        }
      }
    ]
  },
  "dynamic_resources": {
    "lds_config": {
      "api_config_source": {
        "api_type": "GRPC",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour"
            }
          }
        ]
      }
    },
    "cds_config": {
      "api_config_source": {
        "api_type": "GRPC",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour"
            }
          }
        ]
      }
    }
  },
  "admin": {
    "access_log_path": "/dev/null",
    "address": {
      "socket_address": {
        "address": "127.0.0.1",
        "port_value": 9001
      }
    }
  }
}`,
		},
		"--stats-address=8.8.8.8 --stats-port=9200": {
			config: BootstrapConfig{
				Path:      "envoy.json",
				Namespace: "testing-ns",
			},
			wantedBootstrapConfig: `{
  "static_resources": {
    "clusters": [
      {
        "name": "contour",
        "alt_stat_name": "testing-ns_contour_8001",
        "type": "STRICT_DNS",
        "connect_timeout": "5s",
        "load_assignment": {
          "cluster_name": "contour",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "127.0.0.1",
                        "port_value": 8001
                      }
                    }
                  }
                }
              ]
            }
          ]
        },
        "circuit_breakers": {
          "thresholds": [
            {
              "priority": "HIGH",
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50
            },
            {
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50
            }
          ]
        },
        "http2_protocol_options": {},
        "upstream_connection_options": {
          "tcp_keepalive": {
            "keepalive_probes": 3,
            "keepalive_time": 30,
            "keepalive_interval": 5
          }
        }
      },
      {
        "name": "service-stats",
        "alt_stat_name": "testing-ns_service-stats_9001",
        "type": "LOGICAL_DNS",
        "connect_timeout": "0.250s",
        "load_assignment": {
          "cluster_name": "service-stats",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "127.0.0.1",
                        "port_value": 9001
                      }
                    }
                  }
                }
              ]
            }
          ]
        }
      }
    ]
  },
  "dynamic_resources": {
    "lds_config": {
      "api_config_source": {
        "api_type": "GRPC",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour"
            }
          }
        ]
      }
    },
    "cds_config": {
      "api_config_source": {
        "api_type": "GRPC",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour"
            }
          }
        ]
      }
    }
  },
  "admin": {
    "access_log_path": "/dev/null",
    "address": {
      "socket_address": {
        "address": "127.0.0.1",
        "port_value": 9001
      }
    }
  }
}`,
		},
		"--envoy-cafile=CA.cert --envoy-client-cert=client.cert --envoy-client-key=client.key": {
			config: BootstrapConfig{
				Path:              "envoy.json",
				Namespace:         "testing-ns",
				GrpcCABundle:      "CA.cert",
				GrpcClientCert:    "client.cert",
				GrpcClientKey:     "client.key",
				SkipFilePathCheck: true,
			},
			wantedBootstrapConfig: `{
  "static_resources": {
    "clusters": [
      {
        "name": "contour",
        "alt_stat_name": "testing-ns_contour_8001",
        "type": "STRICT_DNS",
        "connect_timeout": "5s",
        "load_assignment": {
          "cluster_name": "contour",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "127.0.0.1",
                        "port_value": 8001
                      }
                    }
                  }
                }
              ]
            }
          ]
        },
        "circuit_breakers": {
          "thresholds": [
            {
              "priority": "HIGH",
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50
            },
            {
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50
            }
          ]
        },
        "http2_protocol_options": {},
        "upstream_connection_options": {
          "tcp_keepalive": {
            "keepalive_probes": 3,
            "keepalive_time": 30,
            "keepalive_interval": 5
          }
        },
        "transport_socket": {
          "name": "envoy.transport_sockets.tls",
          "typed_config": {
            "@type":"type.googleapis.com/envoy.api.v2.auth.UpstreamTlsContext",
            "common_tls_context": {
              "tls_certificates": [
                {
                  "certificate_chain": {
                    "filename": "client.cert"
                  },
                  "private_key": {
                    "filename": "client.key"
                  }
                }
              ],
              "validation_context": {
                "trusted_ca": {
                  "filename": "CA.cert"
                },
		"match_subject_alt_names": [
		  {
		    "exact": "contour"
		  }
                ]
              }
            }
          }
        }
      },
      {
        "name": "service-stats",
        "alt_stat_name": "testing-ns_service-stats_9001",
        "type": "LOGICAL_DNS",
        "connect_timeout": "0.250s",
        "load_assignment": {
          "cluster_name": "service-stats",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "127.0.0.1",
                        "port_value": 9001
                      }
                    }
                  }
                }
              ]
            }
          ]
        }
      }
    ]
  },
  "dynamic_resources": {
    "lds_config": {
      "api_config_source": {
        "api_type": "GRPC",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour"
            }
          }
        ]
      }
    },
    "cds_config": {
      "api_config_source": {
        "api_type": "GRPC",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour"
            }
          }
        ]
      }
    }
  },
  "admin": {
    "access_log_path": "/dev/null",
    "address": {
      "socket_address": {
        "address": "127.0.0.1",
        "port_value": 9001
      }
    }
  }
}`,
		},
		"--resources-dir tmp --envoy-cafile=CA.cert --envoy-client-cert=client.cert --envoy-client-key=client.key": {
			config: BootstrapConfig{
				Path:              "envoy.json",
				Namespace:         "testing-ns",
				ResourcesDir:      "resources",
				GrpcCABundle:      "CA.cert",
				GrpcClientCert:    "client.cert",
				GrpcClientKey:     "client.key",
				SkipFilePathCheck: true,
			},
			wantedBootstrapConfig: `{
        "static_resources": {
          "clusters": [
            {
              "name": "contour",
              "alt_stat_name": "testing-ns_contour_8001",
              "type": "STRICT_DNS",
              "connect_timeout": "5s",
              "load_assignment": {
                "cluster_name": "contour",
                "endpoints": [
                  {
                    "lb_endpoints": [
                      {
                        "endpoint": {
                          "address": {
                            "socket_address": {
                              "address": "127.0.0.1",
                              "port_value": 8001
                            }
                          }
                        }
                      }
                    ]
                  }
                ]
              },
              "circuit_breakers": {
                "thresholds": [
                  {
                    "priority": "HIGH",
                    "max_connections": 100000,
                    "max_pending_requests": 100000,
                    "max_requests": 60000000,
                    "max_retries": 50
                  },
                  {
                    "max_connections": 100000,
                    "max_pending_requests": 100000,
                    "max_requests": 60000000,
                    "max_retries": 50
                  }
                ]
              },
              "http2_protocol_options": {},
              "transport_socket": {
                "name": "envoy.transport_sockets.tls",
                "typed_config": {
                  "@type": "type.googleapis.com/envoy.api.v2.auth.UpstreamTlsContext",
                  "common_tls_context": {
                    "tls_certificate_sds_secret_configs": [
                      {
                        "sds_config": {
                          "path": "resources/sds/xds-tls-certicate.json"
                        }
                      }
                    ],
                    "validation_context_sds_secret_config": {
                      "sds_config": {
                        "path": "resources/sds/xds-validation-context.json"
                      }
                    }
                  }
                }
              },
              "upstream_connection_options": {
                "tcp_keepalive": {
                  "keepalive_probes": 3,
                  "keepalive_time": 30,
                  "keepalive_interval": 5
                }
              }
            },
            {
              "name": "service-stats",
              "alt_stat_name": "testing-ns_service-stats_9001",
              "type": "LOGICAL_DNS",
              "connect_timeout": "0.250s",
              "load_assignment": {
                "cluster_name": "service-stats",
                "endpoints": [
                  {
                    "lb_endpoints": [
                      {
                        "endpoint": {
                          "address": {
                            "socket_address": {
                              "address": "127.0.0.1",
                              "port_value": 9001
                            }
                          }
                        }
                      }
                    ]
                  }
                ]
              }
            }
          ]
        },
        "dynamic_resources": {
          "lds_config": {
            "api_config_source": {
              "api_type": "GRPC",
              "grpc_services": [
                {
                  "envoy_grpc": {
                    "cluster_name": "contour"
                  }
                }
              ]
            }
          },
          "cds_config": {
            "api_config_source": {
              "api_type": "GRPC",
              "grpc_services": [
                {
                  "envoy_grpc": {
                    "cluster_name": "contour"
                  }
                }
              ]
            }
          }
        },
        "admin": {
          "access_log_path": "/dev/null",
          "address": {
            "socket_address": {
              "address": "127.0.0.1",
              "port_value": 9001
            }
          }
        }
    }`,
			wantedTLSCertificateConfig: `{
      "resources": [
        {
          "@type": "type.googleapis.com/envoy.api.v2.auth.Secret",
          "tls_certificate": {
            "certificate_chain": {
              "filename": "client.cert"
            },
            "private_key": {
              "filename": "client.key"
            }
          }
        }
      ]
    }`,
			wantedValidationContextConfig: `{
      "resources": [
        {
          "@type": "type.googleapis.com/envoy.api.v2.auth.Secret",
          "validation_context": {
            "trusted_ca": {
              "filename": "CA.cert"
            },
            "match_subject_alt_names": [
              {
                "exact": "contour"
              }
            ]
          }
        }
      ]
    }`,
		},
		"return error when not providing all certificate related parameters": {
			config: BootstrapConfig{
				Path:           "envoy.json",
				Namespace:      "testing-ns",
				ResourcesDir:   "resources",
				GrpcClientCert: "client.cert",
				GrpcClientKey:  "client.key",
			},
			wantedError: true,
		}}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			steps, gotError := bootstrap(&tc.config)
			assert.Equal(t, gotError != nil, tc.wantedError)

			gotConfigs := map[string]proto.Message{}
			for _, step := range steps {
				path, config := step(&tc.config)
				gotConfigs[path] = config
			}

			sdsTLSCertificatePath := path.Join(tc.config.ResourcesDir, sdsResourcesSubdirectory, sdsTLSCertificateFile)
			sdsValidationContextPath := path.Join(tc.config.ResourcesDir, sdsResourcesSubdirectory, sdsValidationContextFile)

			if tc.wantedBootstrapConfig != "" {
				want := new(envoy_api_bootstrap.Bootstrap)
				unmarshal(t, tc.wantedBootstrapConfig, want)
				assert.Equal(t, want, gotConfigs[tc.config.Path])
				delete(gotConfigs, tc.config.Path)
			}

			if tc.wantedTLSCertificateConfig != "" {
				want := new(api.DiscoveryResponse)
				unmarshal(t, tc.wantedTLSCertificateConfig, want)
				assert.Equal(t, want, gotConfigs[sdsTLSCertificatePath])
				delete(gotConfigs, sdsTLSCertificatePath)
			}

			if tc.wantedValidationContextConfig != "" {
				want := new(api.DiscoveryResponse)
				unmarshal(t, tc.wantedValidationContextConfig, want)
				assert.Equal(t, want, gotConfigs[sdsValidationContextPath])
				delete(gotConfigs, sdsValidationContextPath)
			}

			if len(gotConfigs) > 0 {
				t.Fatalf("got more configs than wanted: %s", gotConfigs)
			}
		})
	}
}

func unmarshal(t *testing.T, data string, pb proto.Message) {
	err := jsonpb.UnmarshalString(data, pb)
	checkErr(t, err)
}

func checkErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
