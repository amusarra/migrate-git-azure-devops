package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// parseElement analizza un singolo elemento (numero o intervallo) e aggiunge
// gli indici zero-based al set seen e alla slice out.
func parseElement(element string, max int, seen map[int]bool, out *[]int) error {
	element = strings.TrimSpace(element)
	if element == "" {
		return nil
	}

	if strings.Contains(element, "-") {
		// Gestione intervallo
		bits := strings.SplitN(element, "-", 2)
		if len(bits) != 2 {
			return fmt.Errorf("intervallo non valido: %s", element)
		}
		a, err1 := strconv.Atoi(strings.TrimSpace(bits[0]))
		b, err2 := strconv.Atoi(strings.TrimSpace(bits[1]))
		if err1 != nil || err2 != nil || a < 1 || b < 1 || a > b || a > max || b > max {
			return fmt.Errorf("intervallo non valido: %s", element)
		}
		for i := a; i <= b; i++ {
			if !seen[i-1] {
				*out = append(*out, i-1)
				seen[i-1] = true
			}
		}
	} else {
		// Gestione numero singolo
		n, err := strconv.Atoi(element)
		if err != nil || n < 1 || n > max {
			return fmt.Errorf("indice non valido: %s", element)
		}
		if !seen[n-1] {
			*out = append(*out, n-1)
			seen[n-1] = true
		}
	}
	return nil
}

// parseSelection converte "1,3-5" in indici zero-based ordinati univoci.
func parseSelection(sel string, max int) ([]int, error) {
	var out []int
	parts := strings.Split(sel, ",")
	seen := map[int]bool{}
	
	for _, p := range parts {
		if err := parseElement(p, max, seen, &out); err != nil {
			return nil, err
		}
	}
	
	sort.Ints(out)
	return out, nil
}
