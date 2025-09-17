package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// parseSelection converte "1,3-5" in indici zero-based ordinati univoci.
func parseSelection(sel string, max int) ([]int, error) {
	var out []int
	parts := strings.Split(sel, ",")
	seen := map[int]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(p, "-") {
			bits := strings.SplitN(p, "-", 2)
			if len(bits) != 2 {
				return nil, fmt.Errorf("intervallo non valido: %s", p)
			}
			a, err1 := strconv.Atoi(strings.TrimSpace(bits[0]))
			b, err2 := strconv.Atoi(strings.TrimSpace(bits[1]))
			if err1 != nil || err2 != nil || a < 1 || b < 1 || a > b || a > max || b > max {
				return nil, fmt.Errorf("intervallo non valido: %s", p)
			}
			for i := a; i <= b; i++ {
				if !seen[i-1] {
					out = append(out, i-1)
					seen[i-1] = true
				}
			}
		} else {
			n, err := strconv.Atoi(p)
			if err != nil || n < 1 || n > max {
				return nil, fmt.Errorf("indice non valido: %s", p)
			}
			if !seen[n-1] {
				out = append(out, n-1)
				seen[n-1] = true
			}
		}
	}
	sort.Ints(out)
	return out, nil
}
