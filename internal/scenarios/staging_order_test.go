package scenarios

// stagingSlot is the place in the staging day of the i-th of n scenarios.
// The library lists related scenarios next to each other, and failures of
// a shared part of the cluster (its DNS, its control plane) are not
// related in time: unrelated outages come at unrelated hours. A stride
// that shares no factor with n visits every slot once and puts library
// neighbours far apart.
func stagingSlot(i, n int) int {
	stride := coprimeStride(n)
	return (i * stride) % n
}

// coprimeStride is the number nearest to n divided by the golden ratio
// that shares no factor with n.
func coprimeStride(n int) int {
	if n < 3 {
		return 1
	}
	stride := int(float64(n) * 0.6180339887)
	for gcd(stride, n) != 1 {
		stride++
	}
	return stride
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
