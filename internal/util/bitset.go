package util

type SmallBitSet struct {
	data uint64
}

func NewSmallBitSet(data uint64) *SmallBitSet {
	return &SmallBitSet{
		data: data,
	}
}

func (b *SmallBitSet) Set(pos int) {
	if pos < 0 {
		return
	}
	b.data |= (1 << pos)
}

func (b *SmallBitSet) Clear(pos int) {
	if pos < 0 || pos >= 64 {
		return
	}
	b.data &^= (1 << pos)
}

func (b *SmallBitSet) Test(pos int) bool {
	if pos < 0 || pos >= 64 {
		return false
	}
	return (b.data & (1 << pos)) != 0
}
