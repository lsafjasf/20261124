package timeline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"strconv"
)

// Merge combines two edits made from the same ancestor.
//
// Independent entity additions, entity edits, and parameter edits are absorbed.
// A property changed to different values on both sides is reported as a
// Conflict. The returned timeline retains the ancestor value at that property
// so the merge never silently prefers "ours", "theirs", or a timestamp.
func Merge(ancestor, ours, theirs Timeline) (MergeResult, error) {
	for _, input := range []struct {
		name     string
		timeline Timeline
	}{
		{name: "ancestor", timeline: ancestor},
		{name: "ours", timeline: ours},
		{name: "theirs", timeline: theirs},
	} {
		if err := input.timeline.Validate(); err != nil {
			return MergeResult{}, wrapMergeError(fmt.Errorf("%s: %w", input.name, err))
		}
	}

	ancestorTree, err := timelineTree(ancestor)
	if err != nil {
		return MergeResult{}, wrapMergeError(err)
	}
	oursTree, err := timelineTree(ours)
	if err != nil {
		return MergeResult{}, wrapMergeError(err)
	}
	theirsTree, err := timelineTree(theirs)
	if err != nil {
		return MergeResult{}, wrapMergeError(err)
	}

	mergedTree, conflicts := mergeNode("", presentNode(ancestorTree), presentNode(oursTree), presentNode(theirsTree))
	if !mergedTree.present {
		return MergeResult{}, wrapMergeError(fmt.Errorf("root timeline was deleted"))
	}
	merged, err := treeTimeline(mergedTree.value)
	if err != nil {
		return MergeResult{}, wrapMergeError(err)
	}
	return MergeResult{Timeline: merged, Conflicts: conflicts}, nil
}

// MergeJSON is the byte-oriented form used by the command line tool.
func MergeJSON(ancestor, ours, theirs []byte) (MergeResult, error) {
	decode := func(label string, data []byte) (Timeline, error) {
		var timeline Timeline
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if err := decoder.Decode(&timeline); err != nil {
			return Timeline{}, wrapMergeError(fmt.Errorf("decode %s: %w", label, err))
		}
		return timeline, nil
	}

	ancestorTimeline, err := decode("ancestor", ancestor)
	if err != nil {
		return MergeResult{}, err
	}
	oursTimeline, err := decode("ours", ours)
	if err != nil {
		return MergeResult{}, err
	}
	theirsTimeline, err := decode("theirs", theirs)
	if err != nil {
		return MergeResult{}, err
	}
	return Merge(ancestorTimeline, oursTimeline, theirsTimeline)
}

type mergeNodeValue struct {
	value   any
	present bool
}

func presentNode(value any) mergeNodeValue {
	return mergeNodeValue{value: value, present: true}
}

func absentNode() mergeNodeValue {
	return mergeNodeValue{}
}

func mergeNode(path string, ancestor, ours, theirs mergeNodeValue) (mergeNodeValue, []Conflict) {
	if nodesEqual(ours, theirs) {
		return ours, nil
	}
	if nodesEqual(ancestor, ours) {
		return theirs, nil
	}
	if nodesEqual(ancestor, theirs) {
		return ours, nil
	}

	if ancestor.present && ours.present && theirs.present {
		if _, ok := ancestor.value.(map[string]any); ok {
			if _, ok := ours.value.(map[string]any); ok {
				if _, ok := theirs.value.(map[string]any); ok {
					return mergeObjects(path, ancestor, ours, theirs)
				}
			}
		}
	}
	if !ancestor.present && ours.present && theirs.present {
		if _, ok := ours.value.(map[string]any); ok {
			if _, ok := theirs.value.(map[string]any); ok {
				return mergeObjects(path, ancestor, ours, theirs)
			}
		}
	}
	if entityList(ancestor) && entityList(ours) && entityList(theirs) {
		return mergeEntityLists(path, ancestor, ours, theirs)
	}

	return ancestor, []Conflict{{
		Path:     path,
		Ancestor: conflictValue(ancestor),
		Ours:     conflictValue(ours),
		Theirs:   conflictValue(theirs),
	}}
}

func mergeObjects(path string, ancestor, ours, theirs mergeNodeValue) (mergeNodeValue, []Conflict) {
	ancestorObject := objectOrEmpty(ancestor)
	oursObject := objectOrEmpty(ours)
	theirsObject := objectOrEmpty(theirs)

	keys := make(map[string]struct{}, len(ancestorObject)+len(oursObject)+len(theirsObject))
	for key := range ancestorObject {
		keys[key] = struct{}{}
	}
	for key := range oursObject {
		keys[key] = struct{}{}
	}
	for key := range theirsObject {
		keys[key] = struct{}{}
	}
	sortedKeys := make([]string, 0, len(keys))
	for key := range keys {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	merged := make(map[string]any, len(sortedKeys))
	var conflicts []Conflict
	for _, key := range sortedKeys {
		childPath := appendObjectKey(path, key)
		child, childConflicts := mergeNode(
			childPath,
			objectValue(ancestorObject, key),
			objectValue(oursObject, key),
			objectValue(theirsObject, key),
		)
		if child.present {
			merged[key] = child.value
		}
		conflicts = append(conflicts, childConflicts...)
	}
	return presentNode(merged), conflicts
}

func mergeEntityLists(path string, ancestor, ours, theirs mergeNodeValue) (mergeNodeValue, []Conflict) {
	ancestorList, ancestorIDs, _ := listEntities(ancestor)
	oursList, oursIDs, _ := listEntities(ours)
	theirsList, theirsIDs, _ := listEntities(theirs)

	ids := make([]string, 0, len(ancestorIDs)+len(oursIDs)+len(theirsIDs))
	seen := make(map[string]struct{}, cap(ids))
	for _, id := range ancestorIDs {
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, id := range oursIDs {
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	for _, id := range theirsIDs {
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}

	merged := make([]any, 0, len(ids))
	var conflicts []Conflict
	for _, id := range ids {
		childPath := appendEntityID(path, id)
		child, childConflicts := mergeNode(
			childPath,
			entityValue(ancestorList, id),
			entityValue(oursList, id),
			entityValue(theirsList, id),
		)
		if child.present {
			merged = append(merged, child.value)
		}
		conflicts = append(conflicts, childConflicts...)
	}
	return presentNode(merged), conflicts
}

func objectValue(object map[string]any, key string) mergeNodeValue {
	value, exists := object[key]
	if !exists {
		return absentNode()
	}
	return presentNode(value)
}

func entityValue(entities map[string]mergeNodeValue, id string) mergeNodeValue {
	value, exists := entities[id]
	if !exists {
		return absentNode()
	}
	return value
}

func objectOrEmpty(node mergeNodeValue) map[string]any {
	if !node.present {
		return nil
	}
	object, _ := node.value.(map[string]any)
	return object
}

func entityList(node mergeNodeValue) bool {
	if !node.present {
		return true
	}
	_, _, ok := listEntities(node)
	return ok
}

func listEntities(node mergeNodeValue) (map[string]mergeNodeValue, []string, bool) {
	if !node.present {
		return map[string]mergeNodeValue{}, nil, true
	}
	items, ok := node.value.([]any)
	if !ok {
		return nil, nil, false
	}

	entities := make(map[string]mergeNodeValue, len(items))
	ids := make([]string, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			return nil, nil, false
		}
		id, ok := objectID(object)
		if !ok {
			return nil, nil, false
		}
		if _, duplicate := entities[id]; duplicate {
			return nil, nil, false
		}
		entities[id] = presentNode(object)
		ids = append(ids, id)
	}
	return entities, ids, true
}

func objectID(object map[string]any) (string, bool) {
	value, exists := object["id"]
	if !exists {
		return "", false
	}
	switch id := value.(type) {
	case string:
		return id, id != ""
	case json.Number:
		return id.String(), id.String() != ""
	default:
		return "", false
	}
}

func nodesEqual(left, right mergeNodeValue) bool {
	if left.present != right.present {
		return false
	}
	if !left.present {
		return true
	}
	return valuesEqual(left.value, right.value)
}

func valuesEqual(left, right any) bool {
	switch leftValue := left.(type) {
	case json.Number:
		rightValue, ok := right.(json.Number)
		return ok && numbersEqual(leftValue, rightValue)
	case map[string]any:
		rightValue, ok := right.(map[string]any)
		if !ok || len(leftValue) != len(rightValue) {
			return false
		}
		for key, value := range leftValue {
			other, exists := rightValue[key]
			if !exists || !valuesEqual(value, other) {
				return false
			}
		}
		return true
	case []any:
		rightValue, ok := right.([]any)
		if !ok || len(leftValue) != len(rightValue) {
			return false
		}
		for i, value := range leftValue {
			if !valuesEqual(value, rightValue[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(left, right)
	}
}

func numbersEqual(left, right json.Number) bool {
	leftRat, leftOK := new(big.Rat).SetString(left.String())
	rightRat, rightOK := new(big.Rat).SetString(right.String())
	if leftOK && rightOK {
		return leftRat.Cmp(rightRat) == 0
	}
	return left.String() == right.String()
}

func appendObjectKey(parent, key string) string {
	if isIdentifier(key) {
		if parent == "" {
			return key
		}
		return parent + "." + key
	}
	return parent + "[" + strconv.Quote(key) + "]"
}

func appendEntityID(parent, id string) string {
	if isIdentifier(id) {
		return parent + "[id=" + id + "]"
	}
	return parent + "[id=" + strconv.Quote(id) + "]"
}

func isIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for i, runeValue := range value {
		if (runeValue >= 'a' && runeValue <= 'z') ||
			(runeValue >= 'A' && runeValue <= 'Z') ||
			runeValue == '_' ||
			(i > 0 && runeValue >= '0' && runeValue <= '9') {
			continue
		}
		return false
	}
	return true
}

func conflictValue(node mergeNodeValue) ConflictValue {
	return ConflictValue{Present: node.present, Value: node.value}
}

func timelineTree(timeline Timeline) (any, error) {
	data, err := json.Marshal(timeline)
	if err != nil {
		return nil, fmt.Errorf("encode timeline: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var tree any
	if err := decoder.Decode(&tree); err != nil {
		return nil, fmt.Errorf("decode canonical timeline: %w", err)
	}
	return tree, nil
}

func treeTimeline(tree any) (Timeline, error) {
	data, err := json.Marshal(tree)
	if err != nil {
		return Timeline{}, fmt.Errorf("encode merged timeline: %w", err)
	}
	var timeline Timeline
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&timeline); err != nil {
		return Timeline{}, fmt.Errorf("decode merged timeline: %w", err)
	}
	return timeline, nil
}
