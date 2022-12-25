package rsync

// func RsyncOperationToPb(op *Operation) pb.Operation {
// 	var opPbType pb.Operation_Type
// 	switch op.Type {
// 	case OpBlock:
// 		opPbType = pb.Operation_OpBlock
// 	case OpData:
// 		opPbType = pb.Operation_OpData
// 	case OpHash:
// 		opPbType = pb.Operation_OpHash
// 	case OpBlockRange:
// 		opPbType = pb.Operation_OpBlockRange
// 	}
//
// 	return pb.Operation{
// 		Type:          opPbType,
// 		BlockIndex:    op.BlockIndex,
// 		BlockIndexEnd: op.BlockIndexEnd,
// 		Data:          op.Data,
// 	}
// }
//
// func PbOperationToRsync(op *pb.Operation) Operation {
// 	var opType OpType
// 	switch op.Type {
// 	case pb.Operation_OpBlock:
// 		opType = OpBlock
// 	case pb.Operation_OpData:
// 		opType = OpData
// 	case pb.Operation_OpHash:
// 		opType = OpHash
// 	case pb.Operation_OpBlockRange:
// 		opType = OpBlockRange
// 	}
//
// 	return Operation{
// 		Type:          opType,
// 		BlockIndex:    op.GetBlockIndex(),
// 		BlockIndexEnd: op.GetBlockIndexEnd(),
// 		Data:          op.GetData(),
// 	}
// }
//
