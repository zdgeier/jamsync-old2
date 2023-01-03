package api

import (
	"bytes"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/zdgeier/jamsync/gen/pb"
	"github.com/zdgeier/jamsync/internal/server/client"
)

func ProjectsHandler(client pb.JamsyncAPIClient) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		resp, err := client.ListProjects(ctx, &pb.ListProjectsRequest{})
		if err != nil {
			ctx.Error(err)
			return
		}
		ctx.JSON(200, resp)
	}
}

func UserProjectsHandler(client pb.JamsyncAPIClient) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		profile := sessions.Default(ctx).Get("profile").(map[string]interface{})
		resp, err := client.ListUserProjects(ctx, &pb.ListUserProjectsRequest{Owner: profile["email"].(string)})
		if err != nil {
			ctx.Error(err)
			return
		}
		ctx.JSON(200, resp)
	}
}

func ProjectBrowseHandler(apiClient pb.JamsyncAPIClient) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		config, err := apiClient.GetProjectConfig(ctx, &pb.GetProjectConfigRequest{
			ProjectName: ctx.Param("projectName"),
		})
		if err != nil {
			ctx.Error(err)
			return
		}
		client := client.NewClient(apiClient, config.GetProjectId(), config.GetCurrentChange())
		resp, err := client.BrowseProject(ctx.Param("path")[1:])
		if err != nil {
			ctx.Error(err)
			return
		}
		ctx.JSON(200, resp)
	}
}

func GetFileHandler(apiClient pb.JamsyncAPIClient) gin.HandlerFunc {
	return func(ctx *gin.Context) {

		config, err := apiClient.GetProjectConfig(ctx, &pb.GetProjectConfigRequest{
			ProjectName: ctx.Param("projectName"),
		})
		if err != nil {
			ctx.Error(err)
			return
		}

		client := client.NewClient(apiClient, config.GetProjectId(), config.GetCurrentChange())

		client.DownloadFile(ctx, ctx.Param("path")[1:], bytes.NewReader([]byte{}), ctx.Writer)
		// resp, err := client.GetFile(ctx, &pb.GetFileRequest{
		// 	ProjectName: ctx.Param("projectName"),
		// 	Path:        ctx.Param("path")[1:],
		// })
		// if err != nil {
		// 	ctx.Error(err)
		// 	return
		// }
	}
}
