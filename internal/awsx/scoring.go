/*
Copyright 2025.

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

package awsx

// import (
// 	"context"
// 	"fmt"
// 	"sort"
// )

// type QuoteScorer struct {
// 	cli        *Client
// 	azNameToID map[string]string
// 	typeScores map[string]map[string]int32
// }

// func NewQuoteScorer(ctx context.Context, cli *Client) (*QuoteScorer, error) {
// 	azMap, err := cli.AZNameToID(ctx)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return &QuoteScorer{
// 		cli:        cli,
// 		azNameToID: azMap,
// 		typeScores: make(map[string]map[string]int32),
// 	}, nil
// }

// // ScoreFor returns the placement score for instanceType in azName.
// func (s *QuoteScorer) ScoreFor(ctx context.Context, instanceType, azName string) (int32, error) {
// 	// Ensure scores for this instance type are cached
// 	if _, ok := s.typeScores[instanceType]; !ok {
// 		m, err := s.cli.PlacementScores(ctx, []string{instanceType}, 1)
// 		if err != nil {
// 			return 0, err
// 		}
// 		s.typeScores[instanceType] = m
// 	}
// 	azID := s.azNameToID[azName]
// 	if azID == "" {
// 		return 0, nil
// 	}
// 	return s.typeScores[instanceType][azID], nil
// }

// // PickCheapestInBatches scans prices in windows of size `window` (e.g., 5) and
// // returns the first quote whose placement score >= threshold.
// // ok=false means none met the threshold; it still returns the absolute cheapest and its score.
// func (s *QuoteScorer) PickCheapestInBatches(ctx context.Context, quotes map[[2]string]SpotQuote, window int, threshold int32) (*SpotQuote, int32, bool, error) {
// 	if window <= 0 {
// 		window = 5
// 	}

// 	// Flatten and sort by price asc
// 	list := make([]SpotQuote, 0, len(quotes))
// 	for _, q := range quotes {
// 		list = append(list, q)
// 	}
// 	sort.Slice(list, func(i, j int) bool { return list[i].PriceUSD < list[j].PriceUSD })

// 	if len(list) == 0 {
// 		return nil, 0, false, nil
// 	}

// 	// Track absolute cheapest in case none meet threshold
// 	cheapest := list[0]
// 	cheapestScore, err := s.ScoreFor(ctx, cheapest.InstanceType, cheapest.Zone)
// 	if err != nil {
// 		return nil, 0, false, err
// 	}

// 	for start := 0; start < len(list); start += window {
// 		end := start + window
// 		end = min(end, len(list))
// 		for i := start; i < end; i++ {
// 			q := list[i]
// 			score, err := s.ScoreFor(ctx, q.InstanceType, q.Zone)
// 			fmt.Println("Score for", q.InstanceType, q.Zone, "is", score)
// 			if err != nil {
// 				return nil, 0, false, err
// 			}
// 			if score >= threshold {
// 				// First meeting the threshold in price order
// 				return &q, score, true, nil
// 			}
// 		}
// 	}
// 	// None met threshold; return absolute cheapest
// 	return &cheapest, cheapestScore, false, nil
// }

import (
	"context"
	"sort"
)

type QuoteScorer struct {
	cli        *Client
	azNameToID map[string]string
	azScores   map[string]int32
}

func NewQuoteScorer(ctx context.Context, cli *Client, instanceTypes []string, targetCount int32) (*QuoteScorer, error) {
	azMap, err := cli.AZNameToID(ctx)
	if err != nil {
		return nil, err
	}
	if targetCount <= 0 {
		targetCount = 1
	}
	scores, err := cli.PlacementScores(ctx, instanceTypes, targetCount)
	if err != nil {
		return nil, err
	}
	return &QuoteScorer{
		cli:        cli,
		azNameToID: azMap,
		azScores:   scores,
	}, nil
}

func (s *QuoteScorer) ScoreFor(ctx context.Context, instanceType, azName string) (int32, error) {
	azID := s.azNameToID[azName]
	if azID == "" {
		return 0, nil
	}
	return s.azScores[azID], nil
}

func (s *QuoteScorer) PickCheapestInBatches(ctx context.Context, quotes map[[2]string]SpotQuote, window int, threshold int32) (*SpotQuote, int32, bool, error) {
	if window <= 0 {
		window = 5
	}

	list := make([]SpotQuote, 0, len(quotes))
	for _, q := range quotes {
		list = append(list, q)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].PriceUSD < list[j].PriceUSD })

	if len(list) == 0 {
		return nil, 0, false, nil
	}

	cheapest := list[0]
	cheapestScore, err := s.ScoreFor(ctx, cheapest.InstanceType, cheapest.Zone)
	if err != nil {
		return nil, 0, false, err
	}

	for start := 0; start < len(list); start += window {
		end := start + window
		if end > len(list) {
			end = len(list)
		}
		for i := start; i < end; i++ {
			q := list[i]
			score, err := s.ScoreFor(ctx, q.InstanceType, q.Zone)
			if err != nil {
				return nil, 0, false, err
			}
			if score >= threshold {
				return &q, score, true, nil
			}
		}
	}
	return &cheapest, cheapestScore, false, nil
}
