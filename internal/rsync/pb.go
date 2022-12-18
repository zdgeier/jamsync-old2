package rsync

import "github.com/zdgeier/jamsync/gen/jamsyncpb"

func RsyncOperationToPb(op *Operation) jamsyncpb.Operation {
	var opPbType jamsyncpb.Operation_Type
	switch op.Type {
	case OpBlock:
		opPbType = jamsyncpb.Operation_OpBlock
	case OpData:
		opPbType = jamsyncpb.Operation_OpData
	case OpHash:
		opPbType = jamsyncpb.Operation_OpHash
	case OpBlockRange:
		opPbType = jamsyncpb.Operation_OpBlockRange
	}

	return jamsyncpb.Operation{
		Type:          opPbType,
		BlockIndex:    op.BlockIndex,
		BlockIndexEnd: op.BlockIndexEnd,
		Data:          op.Data,
	}
}

func PbOperationToRsync(op *jamsyncpb.Operation) Operation {
	var opType OpType
	switch op.Type {
	case jamsyncpb.Operation_OpBlock:
		opType = OpBlock
	case jamsyncpb.Operation_OpData:
		opType = OpData
	case jamsyncpb.Operation_OpHash:
		opType = OpHash
	case jamsyncpb.Operation_OpBlockRange:
		opType = OpBlockRange
	}

	return Operation{
		Type:          opType,
		BlockIndex:    op.GetBlockIndex(),
		BlockIndexEnd: op.GetBlockIndexEnd(),
		Data:          op.GetData(),
	}
}
