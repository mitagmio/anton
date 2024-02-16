package parser

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"math/big"
	"reflect"

	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"github.com/uptrace/bun/extra/bunbig"

	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/ton/nft"
	"github.com/xssnick/tonutils-go/tvm/cell"

	"github.com/tonindexer/anton/abi"
	"github.com/tonindexer/anton/abi/known"
	"github.com/tonindexer/anton/addr"
	"github.com/tonindexer/anton/internal/app"
	"github.com/tonindexer/anton/internal/core"
)

func getMethodByName(i *core.ContractInterface, n string) *abi.GetMethodDesc {
	for it := range i.GetMethodsDesc {
		if i.GetMethodsDesc[it].Name == n {
			return &i.GetMethodsDesc[it]
		}
	}
	return nil
}

func (s *Service) callGetMethod(ctx context.Context, d *abi.GetMethodDesc, acc *core.AccountState, args []any) (ret abi.GetMethodExecution, err error) {
	var argsStack abi.VmStack

	if len(acc.Code) == 0 || len(acc.Data) == 0 {
		return ret, errors.Wrap(app.ErrImpossibleParsing, "no account code or data")
	}

	if len(d.Arguments) != len(args) {
		return ret, errors.New("length of passed and described arguments does not match")
	}
	for it := range args {
		argsStack = append(argsStack, abi.VmValue{
			VmValueDesc: d.Arguments[it],
			Payload:     args[it],
		})
	}

	codeBase64, dataBase64, librariesBase64 :=
		base64.StdEncoding.EncodeToString(acc.Code),
		base64.StdEncoding.EncodeToString(acc.Data),
		base64.StdEncoding.EncodeToString(acc.Libraries)

	e, err := abi.NewEmulatorBase64(acc.Address.MustToTonutils(), codeBase64, dataBase64, s.bcConfigBase64, librariesBase64)
	if err != nil {
		return ret, errors.Wrap(err, "new emulator")
	}

	retStack, err := e.RunGetMethod(ctx, d.Name, argsStack, d.ReturnValues)

	ret = abi.GetMethodExecution{
		Name: d.Name,
	}
	for i := range argsStack {
		ret.Receives = append(ret.Receives, argsStack[i].Payload)
	}
	for i := range retStack {
		ret.Returns = append(ret.Returns, retStack[i].Payload)
	}
	if err != nil {
		ret.Error = err.Error()

		lvl := log.Warn()
		if d.Name == "get_telemint_auction_state" && ret.Error == "tvm execution failed with code 219" {
			lvl = log.Debug() // err::no_auction
		}
		lvl.Err(err).
			Str("get_method", d.Name).
			Str("address", acc.Address.Base64()).
			Int32("workchain", acc.Workchain).
			Int64("shard", acc.Shard).
			Uint32("block_seq_no", acc.BlockSeqNo).
			Msg("run get method")
	}
	return ret, nil
}

func (s *Service) callGetMethodNoArgs(ctx context.Context, i *core.ContractInterface, gmName string, acc *core.AccountState) (ret abi.GetMethodExecution, err error) {
	gm := getMethodByName(i, gmName)
	if gm == nil {
		// we panic as contract interface was defined, but there are no standard get-method
		panic(fmt.Errorf("%s `%s` get-method was not found", i.Name, gmName))
	}
	if len(gm.Arguments) != 0 {
		// we panic as get-method has the wrong description and dev must fix this bug
		panic(fmt.Errorf("%s `%s` get-method has arguments", i.Name, gmName))
	}

	stack, err := s.callGetMethod(ctx, gm, acc, nil)
	if err != nil {
		return ret, errors.Wrapf(err, "%s `%s`", i.Name, gmName)
	}

	return stack, nil
}

func (s *Service) checkPrevGetMethodExecutionArgs(argsDesc []abi.VmValueDesc, args, prevArgs []any) bool { //nolint:gocognit,gocyclo // that's ok
	if len(argsDesc) != len(args) || len(args) != len(prevArgs) {
		return false
	}

	for it := range argsDesc {
		argDesc := &argsDesc[it]

		switch argDesc.StackType {
		case "int":
			argCasted, ok := args[it].(*big.Int)
			if !ok {
				return false
			}

			var prevArgBI *big.Int
			switch argDesc.Format {
			case "":
				switch prevArgCasted := prevArgs[it].(type) {
				case string:
					prevArgBI, ok = new(big.Int).SetString(prevArgCasted, 10)
					if !ok {
						return false
					}
				case float64:
					prevArgBI = big.NewInt(int64(prevArgCasted))
				default:
					return false
				}

			case "bytes":
				prevArgCasted, ok := prevArgs[it].(string)
				if !ok {
					return false
				}
				prevArgCastedBytes, err := base64.StdEncoding.DecodeString(prevArgCasted)
				if err != nil {
					return false
				}
				prevArgBI = new(big.Int).SetBytes(prevArgCastedBytes)

			default:
				return false
			}

			if argCasted.Cmp(prevArgBI) != 0 {
				return false
			}

		case "slice":
			if argDesc.Format != "addr" {
				return false
			}

			argCasted, ok := args[it].(*address.Address)
			if !ok {
				return false
			}

			prevArgCasted, ok := prevArgs[it].(string)
			if !ok {
				return false
			}
			prevArgAddr, err := new(addr.Address).FromBase64(prevArgCasted)
			if err != nil {
				return false
			}

			if !addr.Equal(prevArgAddr, addr.MustFromTonutils(argCasted)) {
				return false
			}

		case "cell":
			if argDesc.Format != "" {
				return false
			}

			argCasted, ok := args[it].(*cell.Cell)
			if !ok {
				return false
			}

			prevArgCasted, ok := prevArgs[it].(string)
			if !ok {
				return false
			}
			prevArgBytes, err := base64.StdEncoding.DecodeString(prevArgCasted)
			if err != nil {
				return false
			}
			prevArgCell, err := cell.FromBOC(prevArgBytes)
			if err != nil {
				return false
			}

			if !bytes.Equal(argCasted.Hash(), prevArgCell.Hash()) {
				return false
			}
		}
	}

	return true
}

// checkPrevGetMethodExecution returns true, if get-method was already executed on that account with the same arguments
func (s *Service) checkPrevGetMethodExecution(i abi.ContractName, desc *abi.GetMethodDesc, acc *core.AccountState, args []any) (int, bool) {
	if acc.ExecutedGetMethods == nil {
		return -1, false
	}

	executions, ok := acc.ExecutedGetMethods[i]
	if !ok {
		return -1, false
	}

	for it := range executions {
		exec := &executions[it]

		if desc.Name != exec.Name {
			continue
		}

		prevDesc := &abi.GetMethodDesc{
			Name:         exec.Name,
			Arguments:    exec.Arguments,
			ReturnValues: exec.ReturnValues,
		}
		if !reflect.DeepEqual(prevDesc, desc) {
			return it, false
		}
		if len(args) == 0 && len(exec.Receives) == 0 {
			return it, true // no arguments, so return values will be the same on second execution
		}

		ok := s.checkPrevGetMethodExecutionArgs(desc.Arguments, args, exec.Receives)
		return it, ok
	}

	return -1, false
}

func (s *Service) removePrevGetMethodExecution(i abi.ContractName, it int, acc *core.AccountState) {
	executions := acc.ExecutedGetMethods[i]
	copy(executions[it:], executions[it+1:])
	acc.ExecutedGetMethods[i] = executions[:len(executions)-1]
}

func mapContentDataNFT(ret *core.AccountState, c any) {
	if c == nil {
		return
	}
	switch content := c.(type) {
	case *nft.ContentSemichain: // TODO: remove this (?)
		ret.ContentURI = content.URI
		ret.ContentName = content.Name
		ret.ContentDescription = content.Description
		ret.ContentImage = content.Image
		ret.ContentImageData = content.ImageData

	case *nft.ContentOnchain:
		ret.ContentName = content.Name
		ret.ContentDescription = content.Description
		ret.ContentImage = content.Image
		ret.ContentImageData = content.ImageData

	case *nft.ContentOffchain:
		ret.ContentURI = content.URI
	}
}

func (s *Service) getNFTItemContent(ctx context.Context, collection *core.AccountState, idx *big.Int, itemContent *cell.Cell, acc *core.AccountState) {
	desc := &abi.GetMethodDesc{
		Name: "get_nft_content",
		Arguments: []abi.VmValueDesc{{
			Name:      "index",
			StackType: "int",
			Format:    "bytes",
		}, {
			Name:      "individual_content",
			StackType: "cell",
		}},
		ReturnValues: []abi.VmValueDesc{{
			Name:      "full_content",
			StackType: "cell",
			Format:    "content",
		}},
	}

	args := []any{idx.Bytes(), itemContent}

	it, valid := s.checkPrevGetMethodExecution(known.NFTCollection, desc, acc, args)
	if valid {
		return // old get-method execution is valid
	}
	if it != -1 {
		s.removePrevGetMethodExecution(known.NFTCollection, it, acc)
	}

	exec, err := s.callGetMethod(ctx, desc, collection, args)
	if err != nil {
		log.Error().Err(err).Msg("execute get_nft_content nft_collection get-method")
		return
	}

	exec.Address = &collection.Address

	acc.ExecutedGetMethods[known.NFTCollection] = append(acc.ExecutedGetMethods[known.NFTCollection], exec)
	if exec.Error != "" {
		return
	}

	mapContentDataNFT(acc, exec.Returns[0])
}

func (s *Service) checkNFTMinter(ctx context.Context, collection *core.AccountState, idx *big.Int, itemAcc *core.AccountState) {
	desc := &abi.GetMethodDesc{
		Name: "get_nft_address_by_index",
		Arguments: []abi.VmValueDesc{{
			Name:      "index",
			StackType: "int",
			Format:    "bytes",
		}},
		ReturnValues: []abi.VmValueDesc{{
			Name:      "address",
			StackType: "slice",
			Format:    "addr",
		}},
	}

	args := []any{idx.Bytes()}

	it, valid := s.checkPrevGetMethodExecution(known.NFTCollection, desc, itemAcc, args)
	if valid {
		return // old get-method execution is valid
	}
	if it != -1 {
		s.removePrevGetMethodExecution(known.NFTCollection, it, itemAcc)
	}

	itemAcc.Fake = true

	exec, err := s.callGetMethod(ctx, desc, collection, args)
	if err != nil {
		log.Error().Err(err).Msg("execute get_nft_address_by_index nft_collection get-method")
		return
	}

	exec.Address = &collection.Address

	itemAcc.ExecutedGetMethods[known.NFTCollection] = append(itemAcc.ExecutedGetMethods[known.NFTCollection], exec)
	if exec.Error != "" {
		log.Error().Err(err).Msg("execute get_nft_address_by_index nft_collection get-method")
		return
	}

	itemAddr := addr.MustFromTonutils(exec.Returns[0].(*address.Address)) //nolint:forcetypeassert // panic on wrong interface
	if addr.Equal(itemAddr, &itemAcc.Address) {
		itemAcc.Fake = false
	}
}

func (s *Service) checkJettonMinter(ctx context.Context, minter *core.AccountState, ownerAddr *addr.Address, walletAcc *core.AccountState) {
	desc := &abi.GetMethodDesc{
		Name: "get_wallet_address",
		Arguments: []abi.VmValueDesc{{
			Name:      "owner_address",
			StackType: "slice",
			Format:    "addr",
		}},
		ReturnValues: []abi.VmValueDesc{{
			Name:      "wallet_address",
			StackType: "slice",
			Format:    "addr",
		}},
	}

	args := []any{ownerAddr.MustToTonutils()}

	it, valid := s.checkPrevGetMethodExecution(known.JettonMinter, desc, walletAcc, args)
	if valid {
		return // old get-method execution is valid
	}
	if it != -1 {
		s.removePrevGetMethodExecution(known.JettonMinter, it, walletAcc)
	}

	walletAcc.Fake = true

	exec, err := s.callGetMethod(ctx, desc, minter, args)
	if err != nil {
		log.Error().Err(err).Msg("execute get_wallet_address jetton_minter get-method")
		return
	}

	exec.Address = &minter.Address

	walletAcc.ExecutedGetMethods[known.JettonMinter] = append(walletAcc.ExecutedGetMethods[known.JettonMinter], exec)
	if exec.Error != "" {
		log.Error().Err(err).Msg("execute get_wallet_address jetton_minter get-method")
		return
	}

	walletAddr := addr.MustFromTonutils(exec.Returns[0].(*address.Address)) //nolint:forcetypeassert // panic on wrong interface
	if addr.Equal(walletAddr, &walletAcc.Address) {
		walletAcc.Fake = false
	}
}

func (s *Service) callPossibleGetMethods( //nolint:gocognit // yeah, it's too long
	ctx context.Context,
	acc *core.AccountState,
	others func(context.Context, addr.Address) (*core.AccountState, error),
	interfaces []*core.ContractInterface,
) {
	for _, i := range interfaces {
		for it := range i.GetMethodsDesc {
			d := &i.GetMethodsDesc[it]

			if len(d.Arguments) != 0 {
				continue
			}

			var (
				exec abi.GetMethodExecution
				err  error
			)

			id, valid := s.checkPrevGetMethodExecution(i.Name, d, acc, nil)
			if valid {
				exec = acc.ExecutedGetMethods[i.Name][id]
			} else {
				if id != -1 {
					s.removePrevGetMethodExecution(i.Name, id, acc)
				}
				exec, err = s.callGetMethodNoArgs(ctx, i, d.Name, acc)
				if err != nil {
					log.Error().Err(err).Str("contract_name", string(i.Name)).Str("get_method", d.Name).Msg("execute get-method")
					return
				}
			}

			acc.ExecutedGetMethods[i.Name] = append(acc.ExecutedGetMethods[i.Name], exec)
			if exec.Error != "" {
				continue
			}

			switch d.Name {
			case "get_collection_data":
				acc.OwnerAddress = addr.MustFromTonutils(exec.Returns[2].(*address.Address)) //nolint:forcetypeassert // panic on wrong interface
				mapContentDataNFT(acc, exec.Returns[1])

			case "get_nft_data":
				acc.MinterAddress = addr.MustFromTonutils(exec.Returns[2].(*address.Address)) //nolint:forcetypeassert // panic on wrong interface
				acc.OwnerAddress = addr.MustFromTonutils(exec.Returns[3].(*address.Address))  //nolint:forcetypeassert // panic on wrong interface

				if acc.MinterAddress == nil {
					continue
				}
				collection, err := others(ctx, *acc.MinterAddress)
				if err != nil {
					log.Error().Str("minter_address", acc.MinterAddress.Base64()).Err(err).Msg("get nft collection state")
					return
				}
				s.getNFTItemContent(ctx, collection, exec.Returns[1].(*big.Int), exec.Returns[4].(*cell.Cell), acc) //nolint:forcetypeassert // panic on wrong interface
				s.checkNFTMinter(ctx, collection, exec.Returns[1].(*big.Int), acc)                                  //nolint:forcetypeassert // panic on wrong interface

			case "get_jetton_data":
				mapContentDataNFT(acc, exec.Returns[3])

			case "get_wallet_data":
				acc.JettonBalance = bunbig.FromMathBig(exec.Returns[0].(*big.Int))            //nolint:forcetypeassert // panic on wrong interface
				acc.OwnerAddress = addr.MustFromTonutils(exec.Returns[1].(*address.Address))  //nolint:forcetypeassert // panic on wrong interface
				acc.MinterAddress = addr.MustFromTonutils(exec.Returns[2].(*address.Address)) //nolint:forcetypeassert // panic on wrong interface

				if acc.MinterAddress == nil || acc.OwnerAddress == nil {
					continue
				}
				minter, err := others(ctx, *acc.MinterAddress)
				if err != nil {
					log.Error().Str("minter_address", acc.MinterAddress.Base64()).Err(err).Msg("get jetton minter state")
					return
				}
				s.checkJettonMinter(ctx, minter, acc.OwnerAddress, acc)
			}
		}
	}
}
