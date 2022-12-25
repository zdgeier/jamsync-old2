package api

import (
	"github.com/gin-gonic/gin"
	"github.com/zdgeier/jamsync/gen/pb"
)

func UserProjectsHandler(client pb.JamsyncAPIClient) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		resp, err := client.ListProjects(ctx, &pb.ListProjectsRequest{})
		if err != nil {
			ctx.Error(err)
			return
		}
		ctx.JSON(200, resp)
	}
}

func ProjectBrowseHandler(client pb.JamsyncAPIClient) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		resp, err := client.BrowseProject(ctx, &pb.BrowseProjectRequest{
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

func GetFileHandler(client pb.JamsyncAPIClient) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// resp, err := client.GetFile(ctx, &pb.GetFileRequest{
		// 	ProjectName: ctx.Param("projectName"),
		// 	Path:        ctx.Param("path")[1:],
		// })
		// if err != nil {
		// 	ctx.Error(err)
		// 	return
		// }
		ctx.Data(200, "", []byte(""))
	}
}
