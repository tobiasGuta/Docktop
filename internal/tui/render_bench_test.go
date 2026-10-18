package tui

import (
	"testing"
)

// These representative read-only render benchmarks distinguish dashboard
// redraw cost from Docker CLI process and daemon overhead.
var renderBenchmarkSink string

func dashboardBenchmarkModel() Model {
	m := testModel()
	m.width, m.height = 172, 52
	m.busy = false
	for i := 0; i < historyLimit; i++ {
		m.cpuHistory = append(m.cpuHistory, float64(i%11)*0.85)
		m.memHistory = append(m.memHistory, 44+float64(i%7)*0.23)
	}
	return m
}

func BenchmarkDashboardView(b *testing.B) {
	m := dashboardBenchmarkModel()
	b.ReportAllocs()
	for i:=0; i<b.N; i++ {
		renderBenchmarkSink = m.View()
	}
}

func BenchmarkBrailleGraphs(b *testing.B) {
	m := dashboardBenchmarkModel()
	b.ReportAllocs()
	for i:=0; i<b.N; i++ {
		_ = plotHistory(m.cpuHistory, 110, 16, 10, green)
		_ = plotHistory(m.memHistory, 70, 8, 8, cyan)
	}
}
