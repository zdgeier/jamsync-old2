package server

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/zdgeier/jamsync/gen/pb"
	"github.com/zdgeier/jamsync/internal/jamenv"
	"github.com/zdgeier/jamsync/internal/server/clientauth"
	"golang.org/x/oauth2"
	"google.golang.org/protobuf/proto"
)

var serverRunning = false

func setup() (pb.JamsyncAPIClient, func(), error) {
	err := jamenv.LoadFile()
	if err != nil {
		return nil, nil, err
	}

	if !serverRunning {
		if jamenv.Env() == jamenv.Local {
			err := os.RemoveAll("jb/")
			if err != nil {
				log.Fatal(err)
			}
			err = os.RemoveAll("jamsync.db")
			if err != nil {
				log.Fatal(err)
			}
		}
		_, err := New()
		if err != nil && err.Error() != "bind: address already in use" {
			return nil, nil, err
		}
		serverRunning = true
	}

	accessToken, err := clientauth.InitConfig()
	if err != nil {
		return nil, nil, err
	}

	client, closer, err := Connect(&oauth2.Token{AccessToken: accessToken})
	if err != nil {
		return nil, nil, err
	}

	return client, func() {
		closer()
		err := os.RemoveAll("jb/")
		if err != nil {
			log.Fatal(err)
		}
		err = os.RemoveAll("jamsync.db")
		if err != nil {
			log.Fatal(err)
		}
	}, nil
}

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
	client, teardown, err := setup()
	require.NoError(t, err)
	defer teardown()

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
