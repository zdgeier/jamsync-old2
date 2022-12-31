package server

import (
	"context"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/zdgeier/jamsync/gen/pb"
	"google.golang.org/protobuf/proto"
)

func TestJamsyncServer_AddListProjects(t *testing.T) {
	type args struct {
		in *pb.AddProjectRequest
	}
	tests := []struct {
		name    string
		args    args
		want    *pb.AddProjectResponse
		wantErr bool
	}{
		{
			name: "Add project 1",
			args: args{
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
			args: args{
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
			args: args{
				in: &pb.AddProjectRequest{
					ProjectName: "test",
				},
			},
			wantErr: true,
		},
	}

	closer, err := New()
	require.NoError(t, err)
	defer closer()

	client, closer, err := Connect()
	require.NoError(t, err)
	defer closer()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			got, err := client.AddProject(ctx, tt.args.in)
			if (err != nil) != tt.wantErr {
				t.Errorf("JamsyncServer.AddProject() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !proto.Equal(got, tt.want) {
				t.Errorf("JamsyncServer.AddProject() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("list projects", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		resp, err := client.ListProjects(ctx, &pb.ListProjectsRequest{})
		require.NoError(t, err)

		want := &pb.ListProjectsResponse{
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
		}
		if !proto.Equal(resp, want) {
			t.Errorf("JamsyncServer.ListProjects() = %v, want %v", resp, want)
		}
	})
}
