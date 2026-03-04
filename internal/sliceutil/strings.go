package sliceutil

func FilterPartition(partitions []string, filter string) []string {
	if filter == "" {
		return partitions
	}
	out := make([]string, 0, 1)
	for _, name := range partitions {
		if name == filter {
			out = append(out, name)
			break
		}
	}
	return out
}

func FilterPartitions(partitions []string, filters []string) []string {
	if len(filters) == 0 {
		return partitions
	}
	allowed := make(map[string]struct{}, len(filters))
	for _, filter := range filters {
		if filter == "" {
			continue
		}
		allowed[filter] = struct{}{}
	}
	if len(allowed) == 0 {
		return partitions
	}
	out := make([]string, 0, len(partitions))
	for _, name := range partitions {
		if _, ok := allowed[name]; ok {
			out = append(out, name)
		}
	}
	return out
}

func DedupeSortedStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	var last string
	for i, item := range items {
		if i == 0 || item != last {
			out = append(out, item)
		}
		last = item
	}
	return out
}

func DedupeStringsPreserveOrder(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
