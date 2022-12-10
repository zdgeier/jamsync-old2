package api

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
)

func UserProjectsHandler(client jamsyncpb.JamsyncAPIClient) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		resp, err := client.ListProjects(ctx, &jamsyncpb.ListProjectsRequest{})
		if err != nil {
			ctx.Error(err)
			return
		}
		ctx.JSON(200, resp)
	}
}

func ProjectBrowseHandler(client jamsyncpb.JamsyncAPIClient) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		resp, err := client.BrowseProject(ctx, &jamsyncpb.BrowseProjectRequest{
			ProjectName: ctx.Param("projectName"),
			Path:        ctx.Param("path")[1:],
		})
		if err != nil {
			ctx.Error(err)
			return
		}
		ctx.JSON(200, resp)
	}
}

func GetFileHandler(client jamsyncpb.JamsyncAPIClient) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		resp, err := client.GetFile(ctx, &jamsyncpb.GetFileRequest{
			ProjectName: ctx.Param("projectName"),
			Path:        ctx.Param("path")[1:],
		})
		if err != nil {
			ctx.Error(err)
			return
		}
		fmt.Println(string(resp.Data))
		ctx.Data(200, "", resp.Data)
	}
}
