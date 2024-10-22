package main

import (
	"bytes"
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

type Utxo struct {
	TxHash        Hash
	Index         uint32
	Value         int64
	PkScript      []byte
	Ancestorfees  int64 //算上未确认的祖父共交的矿工费用 比如：25*5334
	Confirmations int64 //确认数
	Ancestorsize  int64 //算上未确认的祖父共同的虚拟大小 比如： 127*25
	Ancestorcount int64 //算上祖父共有多少笔未确认， 比如：25
}

func (u *Utxo) OutPoint() wire.OutPoint {
	hashBytes := reverseBytes(u.TxHash[:])

	h, err := chainhash.NewHash(hashBytes)
	if err != nil {
		p.Println("Error converting TxHash:", err)
		return wire.OutPoint{}
	}

	return wire.OutPoint{
		Hash:  *h,
		Index: u.Index,
	}
}

func reverseBytes(b []byte) []byte {
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return b
}

func (u *Utxo) TxOut() *wire.TxOut {
	return wire.NewTxOut(u.Value, u.PkScript)
}

type UtxoList []*Utxo

func (l UtxoList) Add(utxo *Utxo) UtxoList {
	return append(l, utxo)
}
func (l UtxoList) FetchPrevOutput(o wire.OutPoint) *wire.TxOut {
	for _, utxo := range l {
		if bytes.Equal(utxo.TxHash[:], o.Hash[:]) && utxo.Index == o.Index {
			return wire.NewTxOut(utxo.Value, utxo.PkScript)
		}
	}
	return nil
}
func (u *Utxo) String() string {
	return fmt.Sprintf("Utxo: {TxHash: %s, Index: %d, Value: %d, PkScript: %x, AncestorFees: %d, Confirmations: %d, AncestorSize: %d, AncestorCount: %d}",
		u.TxHash, u.Index, u.Value, u.PkScript, u.Ancestorfees, u.Confirmations, u.Ancestorsize, u.Ancestorcount)
}
