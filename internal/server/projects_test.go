package server

import (
	"context"
	"database/sql"
	"log"
	"reflect"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/zdgeier/jamsync/gen/pb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/store"
)

func TestJamsyncServer_AddListProjects(t *testing.T) {
	localDB, err := sql.Open("sqlite3", "file:foobar?mode=memory&cache=shared")
	if err != nil {
		log.Panic(err)
	}
	db.Setup(localDB)
	ctx := context.Background()
	memStore := store.NewMemoryStore()

	type fields struct {
		db    *sql.DB
		store store.Store
	}
	type args struct {
		ctx context.Context
		in  *pb.AddProjectRequest
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *pb.AddProjectResponse
		wantErr bool
	}{
		{
			name: "Add project 1",
			fields: fields{
				db:    localDB,
				store: memStore,
			},
			args: args{
				ctx: ctx,
				in: &pb.AddProjectRequest{
					ProjectName: "test",
				},
			},
			want: &pb.AddProjectResponse{
				ProjectId: 1,
			},
		},
		{
			name: "Add project 2",
			fields: fields{
				db:    localDB,
				store: memStore,
			},
			args: args{
				ctx: ctx,
				in: &pb.AddProjectRequest{
					ProjectName: "test2",
				},
			},
			want: &pb.AddProjectResponse{
				ProjectId: 2,
			},
		},
		{
			name: "Add project same name",
			fields: fields{
				db:    localDB,
				store: store.NewMemoryStore(),
			},
			args: args{
				ctx: ctx,
				in: &pb.AddProjectRequest{
					ProjectName: "test",
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := JamsyncServer{
				db:    tt.fields.db,
				store: tt.fields.store,
			}
			got, err := s.AddProject(tt.args.ctx, tt.args.in)
			if (err != nil) != tt.wantErr {
				t.Errorf("JamsyncServer.AddProject() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("JamsyncServer.AddProject() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("list projects", func(t *testing.T) {
		s := JamsyncServer{
			db:    localDB,
			store: memStore,
		}
		resp, err := s.ListProjects(ctx, &pb.ListProjectsRequest{})
		require.NoError(t, err)

		require.Equal(t, resp, &pb.ListProjectsResponse{
			Projects: []*pb.ListProjectsResponse_Project{
				{
					Name: "test",
					Id:   1,
				},
				{
					Name: "test2",
					Id:   2,
				},
			},
		})
	})
}