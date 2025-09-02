package awsx

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type Factory struct{}

func NewFactory() *Factory { return &Factory{} }

type Client struct {
	EC2 *ec2.Client
	// add Pricing if you later need OD price
}

func (f *Factory) ForRegion(ctx context.Context, region string) (*Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, err
	}
	return &Client{
		EC2: ec2.NewFromConfig(cfg),
	}, nil
}

type InstanceMeta struct {
	Type      string
	VCPUs     int32
	MemoryMiB int32
	GPUCount  int32
	GPUMemMiB int32
}

type SpotQuote struct {
	InstanceType string
	Zone         string
	PriceUSD     float64
	Timestamp    time.Time
}

// ListGPUInstanceTypes returns instance types (filtered by families and min GPUs) and their meta.
func (c *Client) ListGPUInstanceTypes(ctx context.Context, families []string, minGPUs int) ([]string, map[string]InstanceMeta, error) {
	p := ec2.NewDescribeInstanceTypesPaginator(c.EC2, &ec2.DescribeInstanceTypesInput{})
	types := []string{}
	meta := make(map[string]InstanceMeta)
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, nil, err
		}
		for _, it := range page.InstanceTypes {
			if it.GpuInfo == nil {
				continue
			}
			var gpuCount int32
			for _, g := range it.GpuInfo.Gpus {
				if g.Count != nil {
					gpuCount += *g.Count
				}
			}
			if gpuCount < int32(minGPUs) || !matchesFamily(string(it.InstanceType), families) {
				continue
			}
			types = append(types, string(it.InstanceType))
			meta[string(it.InstanceType)] = InstanceMeta{
				Type:      string(it.InstanceType),
				VCPUs:     int32(*it.VCpuInfo.DefaultCores),
				MemoryMiB: int32(*it.MemoryInfo.SizeInMiB),
				GPUCount:  gpuCount,
				GPUMemMiB: int32(*it.GpuInfo.TotalGpuMemoryInMiB),
			}
		}
	}
	return types, meta, nil
}

func matchesFamily(instanceType string, families []string) bool {
	if len(families) == 0 {
		return true
	}
	for _, family := range families {
		if strings.HasPrefix(instanceType, family) {
			return true
		}
	}
	return false
}

// LatestSpotPrices gets last N minutes of quotes and keeps latest per (type, zone).
// LatestSpotPrices gets last N minutes of quotes and keeps latest per (type, zone).
func (c *Client) LatestSpotPrices(ctx context.Context, instanceTypes []string, window time.Duration) (map[[2]string]SpotQuote, error) {
	if window <= 0 {
		window = 15 * time.Minute
	}
	now := time.Now().UTC()
	start := now.Add(-window)

	// Convert instance type strings to SDK enum values
	var typeFilters []types.InstanceType
	for _, it := range instanceTypes {
		if it == "" {
			continue
		}
		typeFilters = append(typeFilters, types.InstanceType(it))
	}

	in := &ec2.DescribeSpotPriceHistoryInput{
		StartTime:           &start,
		EndTime:             &now,
		ProductDescriptions: []string{"Linux/UNIX (Amazon VPC)"},
	}
	if len(typeFilters) > 0 {
		in.InstanceTypes = typeFilters
	}

	p := ec2.NewDescribeSpotPriceHistoryPaginator(c.EC2, in)

	latest := make(map[[2]string]SpotQuote, 64)
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, sp := range page.SpotPriceHistory {
			// InstanceType is a value, others are pointers
			it := string(sp.InstanceType)
			if it == "" || sp.AvailabilityZone == nil || sp.Timestamp == nil || sp.SpotPrice == nil {
				continue
			}
			az := *sp.AvailabilityZone
			ts := *sp.Timestamp

			price, err := strconv.ParseFloat(*sp.SpotPrice, 64)
			if err != nil {
				continue
			}

			key := [2]string{it, az}
			if prev, ok := latest[key]; !ok || ts.After(prev.Timestamp) {
				latest[key] = SpotQuote{
					InstanceType: it,
					Zone:         az,
					PriceUSD:     price,
					Timestamp:    ts,
				}
			}
		}
	}

	return latest, nil
}

// PlacementScores returns a simple AZ -> score map (1..10, 0 if unknown).
func (c *Client) PlacementScores(ctx context.Context, instanceTypes []string, targetCount int32) (map[string]int32, error) {
	// TODO: GetSpotPlacementScores with InstanceTypes, TargetCapacity=targetCount
	// Either region-level or AZ-level depending on API; map it to AZ score if available, else return region score.
	return nil, nil
}
