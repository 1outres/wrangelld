package main

import (
	_ "github.com/cilium/ebpf"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go --go-package xdp --output-dir ../xdp xdp ../xdp/xdp.c
