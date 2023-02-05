package repository_test

import (
	"fmt"
	"math/rand"

	"github.com/iam047801/tonidx/internal/core"
)

func randBytes(len int) []byte {
	token := make([]byte, len)
	rand.Read(token)
	return token
}

func randAddr() string {
	return fmt.Sprintf("0:%x", randBytes(32))
}

func randUint() uint64 {
	return rand.Uint64()
}

func randTs() uint64 {
	return randUint()
}

func randLT() uint64 {
	return randUint()
}

var (
	master = &core.Block{
		BlockID: core.BlockID{
			Workchain: -1,
			Shard:     2222,
			SeqNo:     1234,
		},

		FileHash: shard.MasterBlockFileHash,
		RootHash: randBytes(32),

		ShardBlockFileHashes: []string{string(shard.MasterBlockFileHash)},

		Transactions: nil,
	}

	shard = &core.Block{
		BlockID: core.BlockID{
			Workchain: 0,
			Shard:     8888,
			SeqNo:     4321,
		},

		FileHash: randBytes(32),
		RootHash: randBytes(32),

		MasterBlockFileHash: randBytes(32),

		Transactions: nil,
	}

	accWallet = &core.AccountState{
		Latest:  true,
		Address: randAddr(),

		IsActive: true,
		Status:   core.Active,
		Balance:  1e9,

		LastTxLT:   randLT(),
		LastTxHash: randBytes(32),

		StateHash: randBytes(32),
		Code:      randBytes(128),
		CodeHash:  randBytes(32),
		Data:      randBytes(128), // parse it
		DataHash:  randBytes(32),

		Types: []string{"wallet"},
	}

	accItem = &core.AccountState{
		Latest:  true,
		Address: randAddr(),

		IsActive: true,
		Status:   core.Active,
		Balance:  1e9,

		LastTxLT:   randLT(),
		LastTxHash: randBytes(32),

		StateHash: randBytes(32),
		Code:      randBytes(128),
		CodeHash:  randBytes(32),
		Data:      randBytes(128), // parse it
		DataHash:  randBytes(32),

		Types: []string{"item"},
	}

	accDataItem = &core.AccountData{
		Address:      accItem.Address,
		LastTxLT:     accItem.LastTxLT,
		LastTxHash:   accItem.LastTxHash,
		StateHash:    accItem.StateHash,
		Types:        accItem.Types,
		OwnerAddress: randAddr(),
		NFTCollectionData: core.NFTCollectionData{
			NextItemIndex: 43,
		},
		NFTRoyaltyData: core.NFTRoyaltyData{
			RoyaltyAddress: randAddr(),
		},
		NFTContentData: core.NFTContentData{
			ContentURI: "git://asdf.t",
		},
		NFTItemData: core.NFTItemData{
			ItemIndex:         42,
			CollectionAddress: randAddr(),
		},
	}

	msgExtWallet = &core.Message{
		Type:          core.ExternalIn,
		Hash:          randBytes(32),
		Incoming:      true,
		TxAddress:     accWallet.Address,
		TxHash:        accWallet.LastTxHash,
		DstAddress:    accWallet.Address,
		Body:          randBytes(128),
		BodyHash:      randBytes(32),
		StateInitCode: nil,
		StateInitData: nil,
		CreatedAt:     randTs(),
		CreatedLT:     accWallet.LastTxLT,
	}

	txOutWallet = &core.Transaction{
		Address: accWallet.Address,
		Hash:    accWallet.LastTxHash,

		BlockWorkchain: shard.Workchain,
		BlockShard:     shard.Shard,
		BlockSeqNo:     shard.SeqNo,

		PrevTxHash: randBytes(32),
		PrevTxLT:   randLT(),

		InMsgHash:    msgExtWallet.Hash,
		InMsg:        nil,
		OutMsgHashes: []string{string(msgOutWallet.Hash)},
		OutMsg:       nil,

		TotalFees:   1e5,
		StateUpdate: nil,
		Description: nil,
		OrigStatus:  core.Active,
		EndStatus:   core.Active,

		CreatedAt: msgExtWallet.CreatedAt,
		CreatedLT: accWallet.LastTxLT,
	}

	msgOutWallet = &core.Message{
		Type:        core.Internal,
		Hash:        randBytes(32),
		SourceHash:  nil,
		Source:      nil,
		Incoming:    false,
		TxAddress:   accWallet.Address,
		TxHash:      accWallet.LastTxHash,
		SrcAddress:  accWallet.Address,
		DstAddress:  accItem.Address,
		Bounce:      false,
		Bounced:     false,
		Amount:      1e5,
		IHRDisabled: false,
		IHRFee:      0,
		FwdFee:      0,
		Body:        randBytes(32),
		BodyHash:    randBytes(32),
		OperationID: 0xffeeee,
		CreatedAt:   msgExtWallet.CreatedAt,
		CreatedLT:   accWallet.LastTxLT + 1,
	}

	msgInItem = &core.Message{
		Type: core.Internal,

		Hash:       randBytes(32),
		SourceHash: msgOutWallet.Hash,

		TxAddress: accItem.Address,
		TxHash:    accItem.LastTxHash,

		Incoming:   true,
		SrcAddress: accWallet.Address,
		DstAddress: accItem.Address,

		Amount: msgOutWallet.Amount,

		IHRDisabled: false,
		IHRFee:      0,
		FwdFee:      0,

		Body:     msgOutWallet.Body,
		BodyHash: msgOutWallet.BodyHash,

		OperationID: msgOutWallet.OperationID,

		CreatedAt: msgOutWallet.CreatedAt + 1,
		CreatedLT: msgOutWallet.CreatedLT,
	}

	txInItem = &core.Transaction{
		Address: accItem.Address,
		Hash:    accItem.LastTxHash,

		BlockWorkchain: shard.Workchain,
		BlockShard:     shard.Shard,
		BlockSeqNo:     shard.SeqNo,

		PrevTxHash: randBytes(32),
		PrevTxLT:   randLT(),

		InMsgHash: msgInItem.Hash,

		TotalFees: 1e3,

		StateUpdate: randBytes(32),
		Description: randBytes(32),

		OrigStatus: core.Active,
		EndStatus:  core.Active,

		CreatedAt: msgInItem.CreatedAt,
		CreatedLT: msgInItem.CreatedLT + 1,
	}

	msgInItemPayload = &core.MessagePayload{
		Type:          core.Internal,
		Hash:          msgInItem.Hash,
		Incoming:      true,
		TxAddress:     accItem.Address,
		TxHash:        msgInItem.TxHash,
		SrcAddress:    msgInItem.SrcAddress,
		SrcContract:   core.ContractType(accWallet.Types[0]),
		DstAddress:    msgInItem.DstAddress,
		DstContract:   core.ContractType(accItem.Types[0]),
		Bounce:        false,
		Bounced:       false,
		Amount:        msgInItem.Amount,
		BodyHash:      msgInItem.BodyHash,
		OperationID:   msgInItem.OperationID,
		OperationName: "item_transfer",
		DataJSON:      "{\"new_owner\": \"kkkkkk\"}",
		CreatedAt:     msgInItem.CreatedAt,
		CreatedLT:     msgInItem.CreatedLT,
	}
)
