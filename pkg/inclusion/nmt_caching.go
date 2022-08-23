package inclusion

import (
	"fmt"
	"math"

	"github.com/celestiaorg/nmt"
	"github.com/celestiaorg/rsmt2d"
	"github.com/tendermint/tendermint/pkg/da"
	"github.com/tendermint/tendermint/pkg/wrapper"
)

// TODO optimize https://github.com/celestiaorg/nmt/blob/e679661c6776d8a694f4f7c423c2e2eccb6c5aaa/subrootpaths.go#L15-L28

// subTreeRootCacher keep track of all the inner nodes of an nmt using a simple
// map. Note: this cacher does not cache individual leaves or their hashes, only
// inner nodes.
type subTreeRootCacher struct {
	cache                  map[string][2]string
	depthCursor, posCurosr int
}

func (c *subTreeRootCacher) coord() (int, int) {
	return c.depthCursor, c.posCurosr
}

func newSubTreeRootCacher(squareSize int) *subTreeRootCacher {
	return &subTreeRootCacher{cache: make(map[string][2]string), depthCursor: int(math.Log2(float64(squareSize * 2)))}
}

// Visit fullfills the nmt.VisitorNode function definition. It stores each inner
// node in a simple map, which can later be used to walk the tree. This function
// is called by the nmt when calculating the root.
func (strc *subTreeRootCacher) Visit(hash []byte, children ...[]byte) {
	switch len(children) {
	case 2:
		strc.cache[string(hash)] = [2]string{string(children[0]), string(children[1])}
	case 1:
		// todo(remove)
		// strc.cache[string(hash)] = [2]string{string(children[0]), string(children[0])}
		return
	default:
		panic("unexpected visit")
	}
}

// walk recursively traverses the subTreeRootCacher's internal tree by using the
// provided sub tree root and path. The provided path should be a []bool, false
// indicating that the first child node (left most node) should be used to find
// the next path, and the true indicating that the second (right) should be used.
// walk throws an error if the sub tree cannot be found.
func (strc subTreeRootCacher) walk(root []byte, path []bool) ([]byte, error) {
	// return if we've reached the end of the path
	if len(path) == 0 {
		return root, nil
	}
	// try to lookup the provided sub root
	children, has := strc.cache[string(root)]
	if !has {
		// note: we might want to consider panicing here
		return nil, fmt.Errorf("did not find sub tree root: %v", root)
	}

	// continue to traverse the tree by recursively calling this function on the next root
	switch path[0] {
	// walk left
	case false:
		return strc.walk([]byte(children[0]), path[1:])
	// walk right
	case true:
		return strc.walk([]byte(children[1]), path[1:])
	default:
		// this is unreachable code, but the compiler doesn't recognize this somehow
		panic("bool other than true or false, computers were a mistake, everything is a lie, math is fake.")
	}
}

// EDSSubTreeRootCacher caches the inner nodes for each row so that we can
// traverse it later to check for message inclusion. NOTE: Currently this has to
// use a leaky abstraction (see docs on counter field below), and is not
// threadsafe, but with a future refactor, we could simply read from rsmt2d and
// not use the tree constructor which would fix both of these issues.
type EDSSubTreeRootCacher struct {
	caches     []*subTreeRootCacher
	squareSize uint64
	// counter is used to ignore columns NOTE: this is a leaky abstraction that
	// we make because rsmt2d is used to generate the roots for us, so we have
	// to assume that it will generate a row root every other tree contructed.
	// This is also one of the reasons this implementation is not thread safe.
	// Please see note above on a better refactor.
	counter int
}

func NewSubtreeCacher(squareSize uint64) *EDSSubTreeRootCacher {
	return &EDSSubTreeRootCacher{
		caches:     []*subTreeRootCacher{},
		squareSize: squareSize,
	}
}

// Constructor fullfills the rsmt2d.TreeCreatorFn by keeping a pointer to the
// cache and embedding it as a nmt.NodeVisitor into a new wrapped nmt. I only
func (stc *EDSSubTreeRootCacher) Constructor() rsmt2d.Tree {
	// see docs of counter field for more
	// info. if the counter is even or == 0, then we make the assumption that we
	// are creating a tree for a row
	var newTree wrapper.ErasuredNamespacedMerkleTree
	switch stc.counter % 2 {
	case 0:
		strc := newSubTreeRootCacher(int(stc.squareSize))
		stc.caches = append(stc.caches, strc)
		newTree = wrapper.NewErasuredNamespacedMerkleTree(stc.squareSize, nmt.NodeVisitor(strc.Visit))
	default:
		newTree = wrapper.NewErasuredNamespacedMerkleTree(stc.squareSize)
	}

	stc.counter++
	return &newTree
}

// GetSubTreeRoot traverses the nmt of the selected row and returns the
// subtree root. An error is thrown if the subtree cannot be found.
func (stc *EDSSubTreeRootCacher) GetSubTreeRoot(dah da.DataAvailabilityHeader, row int, path []bool) ([]byte, error) {
	const unexpectedDAHErr = "data availability header has unexpected number of row roots: expected %d got %d"
	if len(stc.caches) != len(dah.RowsRoots) {
		return nil, fmt.Errorf(unexpectedDAHErr, len(stc.caches), len(dah.RowsRoots))
	}
	const rowOutOfBoundsErr = "row exceeds range of cache: max %d got %d"
	if row >= len(stc.caches) {
		return nil, fmt.Errorf(rowOutOfBoundsErr, len(stc.caches), row)
	}
	return stc.caches[row].walk(dah.RowsRoots[row], path)
}

// todo delete
func (stc *EDSSubTreeRootCacher) Debug() {
	// for i, cache := range stc.caches {
	// }
}
