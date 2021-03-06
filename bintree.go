// Copyright 2018 Couchbase, Inc. All rights reserved.

package gojsonsm

import (
	"errors"
	"fmt"
	"strings"
)

type BinTreeNodeType int

const (
	nodeTypeLeaf BinTreeNodeType = iota
	nodeTypeOr
	nodeTypeAnd
	nodeTypeNot
	nodeTypeNeor
	nodeTypeLoop
)

func binTreeNodeTypeToString(nodeType BinTreeNodeType) string {
	switch nodeType {
	case nodeTypeLeaf:
		return "leaf"
	case nodeTypeOr:
		return "or"
	case nodeTypeAnd:
		return "and"
	case nodeTypeNot:
		return "not"
	case nodeTypeNeor:
		return "neor"
	case nodeTypeLoop:
		return "loop"
	}
	return "??ERROR??"
}

func binTreeNodeTypeHasLeft(nodeType BinTreeNodeType) bool {
	return nodeType != nodeTypeLeaf
}

func binTreeNodeTypeHasRight(nodeType BinTreeNodeType) bool {
	return nodeType != nodeTypeLeaf && nodeType != nodeTypeNot && nodeType != nodeTypeLoop
}

type binTreePointers struct {
	ParentIdx int
	Left      int
	Right     int
}

type binTreeNode struct {
	binTreePointers
	NodeType BinTreeNodeType
}

func NewBinTreeNode(nodeType BinTreeNodeType, parent, left, right int) *binTreeNode {
	node := &binTreeNode{
		NodeType: nodeType,
	}
	node.ParentIdx = parent
	node.Left = left
	node.Right = right
	return node
}

type binParserTreeNode struct {
	binTreePointers
	tokenType ParseTokenType
}

type binParserTree struct {
	data []binParserTreeNode
}

type binTree struct {
	data []binTreeNode
}

func (tree binTree) itemToString(item int) string {
	var out string
	idata := tree.data[item]
	out += fmt.Sprintf("[%d:%d] %s\n", item, idata.ParentIdx, binTreeNodeTypeToString(idata.NodeType))
	if idata.Left != 0 {
		out += reindentString(tree.itemToString(idata.Left), "  ") + "\n"
	}
	if idata.Right != 0 {
		out += reindentString(tree.itemToString(idata.Right), "  ") + "\n"
	}
	return strings.TrimRight(out, "\n")
}

func (tree binTree) String() string {
	return tree.itemToString(0)
}

type binTreeStateValue int

const (
	binTreeStateUnknown binTreeStateValue = iota
	binTreeStateResolved
	binTreeStateTrue
	binTreeStateFalse
)

type binTreeState struct {
	tree       *binTree
	data       []binTreeStateValue
	stallIndex int
}

func (state binTreeState) itemToString(item int) string {
	var out string
	idata := state.tree.data[item]
	istate := state.data[item]

	switch istate {
	case binTreeStateUnknown:
		out += fmt.Sprintf("[%d] %s\n", item, binTreeNodeTypeToString(idata.NodeType))
	case binTreeStateResolved:
		out += fmt.Sprintf("[%d] %s = undefined\n", item, binTreeNodeTypeToString(idata.NodeType))
	case binTreeStateTrue:
		out += fmt.Sprintf("[%d] %s = true\n", item, binTreeNodeTypeToString(idata.NodeType))
	case binTreeStateFalse:
		out += fmt.Sprintf("[%d] %s = false\n", item, binTreeNodeTypeToString(idata.NodeType))
	}

	if idata.Left != 0 {
		out += reindentString(state.itemToString(idata.Left), "  ") + "\n"
	}
	if idata.Right != 0 {
		out += reindentString(state.itemToString(idata.Right), "  ") + "\n"
	}
	return strings.TrimRight(out, "\n")
}

func (state binTreeState) String() string {
	return state.itemToString(0)
}

func (tree *binTree) NewState() *binTreeState {
	return &binTreeState{
		tree: tree,
		data: make([]binTreeStateValue, len(tree.data)),
	}
}

func (tree *binTree) validateItem(item int, parent int) (int, error) {
	idata := tree.data[item]

	if idata.ParentIdx != parent {
		return -1, errors.New("parent index doesnt match child")
	}

	switch idata.NodeType {
	case nodeTypeLeaf:
		// Leafs should not have any children
		return item + 1, nil
	case nodeTypeAnd:
	case nodeTypeOr:
	case nodeTypeNot:
	case nodeTypeNeor:
	case nodeTypeLoop:
	default:
		// Invalid node type
		return -1, errors.New("unexpected node type")
	}

	if !binTreeNodeTypeHasLeft(idata.NodeType) {
		// Left must not be set
		if idata.Left != 0 {
			return -1, errors.New("expected left to be undefined")
		}
	} else {
		// Left must be set, and be inside the tree
		if idata.Left <= 0 || idata.Left >= len(tree.data) {
			return -1, errors.New("expected left to be defined, and inside the tree")
		}
	}

	if !binTreeNodeTypeHasRight(idata.NodeType) {
		// Right must not be set
		if idata.Right != 0 {
			return -1, errors.New("expected right to be undefined")
		}
	} else {
		// Right must be set, and be inside the tree
		if idata.Right <= 0 || idata.Right >= len(tree.data) {
			return -1, errors.New("expected right to be defined, and inside the tree")
		}
	}

	// Check the children
	var err error
	pos := item + 1

	if binTreeNodeTypeHasLeft(idata.NodeType) {
		pos, err = tree.validateItem(pos, item)
		if err != nil {
			return -1, err
		}
	}

	if binTreeNodeTypeHasRight(idata.NodeType) {
		pos, err = tree.validateItem(pos, item)
		if err != nil {
			return -1, err
		}
	}

	return pos, nil
}

func (tree *binTree) NumNodes() int {
	return len(tree.data)
}

func (tree *binTree) Validate() error {
	pos, err := tree.validateItem(0, 0)
	if err != nil {
		return err
	}

	if pos != len(tree.data) {
		return errors.New("tree does not encompase all nodes")
	}

	return nil
}

func (tree *binParserTree) NumNodes() int {
	return len(tree.data)
}

func (state *binTreeState) CopyFrom(ostate *binTreeState) {
	if state.tree != ostate.tree {
		panic("cannot copy from different tree")
	}

	for i, item := range ostate.data {
		state.data[i] = item
	}
}

func (state *binTreeState) SetStallIndex(index int) int {
	oldStallIndex := state.stallIndex
	state.stallIndex = index
	return oldStallIndex
}

// now sure how this works. it looks like it is marking leaf nodes as false,
// then go up the tree. Why would this work?
// Resolve forces the tree to be fully resolved (including cases such as NOT)
// by doing a depth-first resolution of all unresolved nodes with `false`.
func (state *binTreeState) Resolve() {
	// Skip resolving if the full tree is already resolved
	if state.IsResolved(0) {
		return
	}

	// Do depth-first resolution of the entire tree state
	treeLength := len(state.data)
	for i := treeLength - 1; i >= 0; i-- {
		// If this bucket is not resolved, resolve it with false
		if state.data[i] == binTreeStateUnknown {
			state.MarkNode(i, false)
		}

		// Leave as soon as the root has been resolved
		if state.data[0] != binTreeStateUnknown {
			break
		}
	}
}

func (state *binTreeState) Reset() {
	state.stallIndex = 0
	for i := range state.data {
		state.data[i] = binTreeStateUnknown
	}
}

func (state *binTreeState) resetNodeRecursive(index int) {
	// TODO(brett19): This is technically quite slow.  It would be ideal if
	// the binary tree itself marked the end of each node so we could do a
	// quick loop through all the entries at once.

	state.data[index] = binTreeStateUnknown

	defNode := state.tree.data[index]
	if binTreeNodeTypeHasLeft(defNode.NodeType) {
		state.resetNodeRecursive(defNode.Left)
	}
	if binTreeNodeTypeHasRight(defNode.NodeType) {
		state.resetNodeRecursive(defNode.Right)
	}
}

func (state *binTreeState) ResetNode(index int) {
	state.resetNodeRecursive(index)
}

func (state *binTreeState) resolveRecursive(index int) {
	defNode := state.tree.data[index]
	if binTreeNodeTypeHasLeft(defNode.NodeType) {
		if state.data[defNode.Left] == binTreeStateUnknown {
			state.data[defNode.Left] = binTreeStateResolved
			state.resolveRecursive(defNode.Left)
		}
	}
	if binTreeNodeTypeHasRight(defNode.NodeType) {
		if state.data[defNode.Right] == binTreeStateUnknown {
			state.data[defNode.Right] = binTreeStateResolved
			state.resolveRecursive(defNode.Right)
		}
	}
}

func (state *binTreeState) checkNode(index int) {
	defNode := state.tree.data[index]
	if defNode.NodeType == nodeTypeLeaf {
		panic("cannot check leaf")
	}

	if defNode.NodeType == nodeTypeOr {
		if state.data[defNode.Left] == binTreeStateTrue || state.data[defNode.Right] == binTreeStateTrue {
			state.MarkNode(index, true)
		} else if state.data[defNode.Left] == binTreeStateFalse && state.data[defNode.Right] == binTreeStateFalse {
			state.MarkNode(index, false)
		}
		return
	} else if defNode.NodeType == nodeTypeNeor {
		if state.data[defNode.Left] != binTreeStateUnknown && state.data[defNode.Right] != binTreeStateUnknown {
			if state.data[defNode.Left] == binTreeStateTrue || state.data[defNode.Right] == binTreeStateTrue {
				state.MarkNode(index, true)
			} else {
				state.MarkNode(index, false)
			}
		}
		return
	} else if defNode.NodeType == nodeTypeAnd {
		if state.data[defNode.Left] == binTreeStateTrue && state.data[defNode.Right] == binTreeStateTrue {
			state.MarkNode(index, true)
		} else if state.data[defNode.Left] == binTreeStateFalse || state.data[defNode.Right] == binTreeStateFalse {
			state.MarkNode(index, false)
		}
		return
	} else if defNode.NodeType == nodeTypeNot {
		if state.data[defNode.Left] == binTreeStateTrue {
			state.MarkNode(index, !true)
		} else if state.data[defNode.Left] == binTreeStateFalse {
			state.MarkNode(index, !false)
		}
		return
	} else if defNode.NodeType == nodeTypeLoop {
		if state.data[defNode.Left] == binTreeStateTrue {
			state.MarkNode(index, true)
		} else if state.data[defNode.Left] == binTreeStateFalse {
			state.MarkNode(index, false)
		}
		return
	}

	panic("invalid node mode")
}

func (state *binTreeState) MarkNode(index int, value bool) {
	if state.data[index] != binTreeStateUnknown {
		panic("cannot resolve same state node twice")
	}

	if value {
		state.data[index] = binTreeStateTrue
	} else {
		state.data[index] = binTreeStateFalse
	}
	state.resolveRecursive(index)

	// We are done if we are the root node
	if index == 0 {
		return
	}

	// If we are the marked stall index, we should stop recursing.
	if index == state.stallIndex {
		return
	}

	// Check for parent satisfaction
	defNode := state.tree.data[index]
	state.checkNode(defNode.ParentIdx)
}

func (state *binTreeState) IsResolved(index int) bool {
	return state.data[index] != binTreeStateUnknown
}

func (state *binTreeState) IsTrue(index int) bool {
	return state.data[index] == binTreeStateTrue
}
