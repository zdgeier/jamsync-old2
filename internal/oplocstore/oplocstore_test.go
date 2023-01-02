package oplocstore

import (
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zdgeier/jamsync/gen/pb"
	"google.golang.org/protobuf/proto"
)

func teardown() {
	err := os.RemoveAll("jbtest/")
	if err != nil {
		log.Fatal(err)
	}
}

func TestOpLocStore(t *testing.T) {
	type fields struct {
		directory     string
		openFileCache map[string]*os.File
	}
	type args struct {
		opLocs *pb.OperationLocations
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
		want    *pb.OperationLocations
	}{
		{
			name: "Sanity",
			fields: fields{
				directory:     "jbtest",
				openFileCache: make(map[string]*os.File),
			},
			args: args{
				opLocs: &pb.OperationLocations{
					ProjectId: 3,
					ChangeId:  4,
					PathHash:  123,
					OpLocs: []*pb.OperationLocations_OperationLocation{
						{
							Offset: 10,
							Length: 20,
						},
						{
							Offset: 20,
							Length: 30,
						},
					},
				},
			},
			wantErr: false,
			want: &pb.OperationLocations{
				ProjectId: 3,
				ChangeId:  4,
				PathHash:  123,
				OpLocs: []*pb.OperationLocations_OperationLocation{
					{
						Offset: 10,
						Length: 20,
					},
					{
						Offset: 20,
						Length: 30,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := LocalOpLocStore{
				directory: tt.fields.directory,
			}
			if err := s.InsertOperationLocations(tt.args.opLocs); (err != nil) != tt.wantErr {
				t.Errorf("LocalOpLocStore.InsertOperationLocations() error = %v, wantErr %v", err, tt.wantErr)
			}

			got, err := s.ListOperationLocations(tt.args.opLocs.GetProjectId(), tt.args.opLocs.GetPathHash(), tt.args.opLocs.GetChangeId())
			require.NoError(t, err)
			require.True(t, proto.Equal(tt.want, got))

			m := MemoryOpLocStore{
				opLocs: make(map[uint64]map[uint64]map[uint64]*pb.OperationLocations),
			}
			if err := m.InsertOperationLocations(tt.args.opLocs); (err != nil) != tt.wantErr {
				t.Errorf("MemoryOpLocStore.InsertOperationLocations() error = %v, wantErr %v", err, tt.wantErr)
			}

			got, err = m.ListOperationLocations(tt.args.opLocs.GetProjectId(), tt.args.opLocs.GetPathHash(), tt.args.opLocs.GetChangeId())
			require.NoError(t, err)
			require.True(t, proto.Equal(tt.want, got))
		})
	}
	teardown()
}
