package main

import (
	"fmt"
	"math"
	"os"
)

type body struct {
	id                        int
	x, y, z, vx, vy, vz, mass float64
}

func initial() [5]body {
	return [5]body{
		{0, 0, 0, 0, 0, 0, 0, 1},
		{1, 1, 0, 0, 0, 1, 0, 0.001},
		{2, 0, 2, 0, -0.7, 0, 0, 0.001},
		{3, -3, 0, 0, 0, -0.57, 0, 0.001},
		{4, 0, -4, 0.1, 0.5, 0, 0, 0.001},
	}
}

func advance(bodies [5]body) [5]body {
	var next [5]body
	for i, b := range bodies {
		for _, other := range bodies {
			dx, dy, dz := other.x-b.x, other.y-b.y, other.z-b.z
			distance2 := dx*dx + dy*dy + dz*dz + 0.01
			scale := 0.001 * other.mass / (distance2 * math.Sqrt(distance2))
			b.vx += dx * scale
			b.vy += dy * scale
			b.vz += dz * scale
		}
		b.x += 0.001 * b.vx
		b.y += 0.001 * b.vy
		b.z += 0.001 * b.vz
		next[i] = b
	}
	return next
}

func energy(bodies [5]body) float64 {
	total := 0.0
	for _, b := range bodies {
		kinetic := 0.5 * b.mass * (b.vx*b.vx + b.vy*b.vy + b.vz*b.vz)
		potential := 0.0
		for _, other := range bodies {
			if b.id == other.id {
				continue
			}
			dx, dy, dz := other.x-b.x, other.y-b.y, other.z-b.z
			potential -= 0.5 * b.mass * other.mass / math.Sqrt(dx*dx+dy*dy+dz*dz+0.01)
		}
		total = total + kinetic + potential
	}
	return total
}

func main() {
	var steps int
	if _, err := fmt.Fscan(os.Stdin, &steps); err != nil {
		panic(err)
	}
	if steps < 0 {
		panic("expected nonnegative steps")
	}
	bodies := initial()
	for range steps {
		bodies = advance(bodies)
	}
	fmt.Printf("%.9f\n", energy(bodies))
}
