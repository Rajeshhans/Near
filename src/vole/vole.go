package main

import (
  "crypto/md5"
  "encoding/json"
  "fmt"
  btsync "github.com/vole/btsync-api"
  "github.com/vole/web"
  "io"
  "io/ioutil"
  "lib/config"
  "lib/store"
  "os"
  osuser "os/user"
  "path"
)

func Md5(r io.Reader) string {
  hash := md5.New()
  io.Copy(hash, r)
  return fmt.Sprintf("%x", hash.Sum(nil))
}

var DIR = func() string {
  dir := "."
  user, err := osuser.Current()
  if err == nil {
    dir = user.HomeDir
  }
  return path.Join(dir, "Vole")
}()

var userStore = &store.UserStore{
  Path:    DIR,
  Version: "v1",
}

var serveIndex = func(ctx *web.Context) string {
  ctx.SetHeader("Content-Security-Policy", "script-src 'self' 'unsafe-eval'", true)
  ctx.SetHeader("Content-Type", "text/html", true)

  index, err := ioutil.ReadFile("static/index.html")
  if err != nil {
    panic(err)
  }

  return string(index)
}

func setJsonHeaders(ctx *web.Context) {
  ctx.ContentType("json")
  ctx.SetHeader("Cache-Control", "no-cache, no-store", true)
}

func main() {
  // Use the fmt package by default so that we don't have to keep commenting it.
  fmt.Println("vole startup")

  config, err := config.Load()
  if err != nil {
    panic(err)
  }

  web.Get("/js/app/config.js", func(ctx *web.Context) string {
    setJsonHeaders(ctx)
    configJson, err := json.Marshal(config)
    if err != nil {
      ctx.Abort(500, "Error marshalling config.")
    }

    return "define(function () { return " + string(configJson) + "; });"
  })

  web.Get("/api/posts", func(ctx *web.Context) string {
    setJsonHeaders(ctx)
    limit := config.UI.PageSize
    before, _ := ctx.Params["before"]
    userId, _ := ctx.Params["user"]

    var allPosts *store.PostCollection
    var err error

    if userId != "" {
      var user *store.User
      if userId == "my_user" {
        user, err = userStore.GetMyUser()
      } else {
        user, err = userStore.GetUserById(userId)
      }
      if err != nil {
        ctx.Abort(500, "User not found while getting posts.")
        return ""
      }
      allPosts, err = user.GetPosts()
      if err != nil {
        ctx.Abort(500, "Error loading posts.")
      }
    } else {
      allPosts, err = userStore.GetPosts()
      if err != nil || len(allPosts.Posts) < 1 {
        // Return a welcome post.
        post := &store.Post{}
        post.InitNew("Welcome to Vole. To start, create a new profile by clicking 'My Profile' on the left.", []store.File{}, "none", "none", "Welcome", "", false)
        post.Id = "none"
        allPosts = post.Collection()
      }
    }

    allPosts.BeforeId(before)
    allPosts.Limit(limit)

    postsJson, err := allPosts.Json()
    if err != nil {
      ctx.Abort(500, "Error getting posts as json.")
    }

    return postsJson
  })

  web.Get("/api/users", func(ctx *web.Context) string {
    setJsonHeaders(ctx)
    _, isMyUserFilter := ctx.Params["is_my_user"]

    var users *store.UserCollection

    if isMyUserFilter {
      myUser, _ := userStore.GetMyUser()
      if myUser != nil {
        users = myUser.Collection()
      } else {
        users = store.GetEmptyUserCollection()
      }
    } else {
      users, err = userStore.GetUsers()
      if err != nil {
        ctx.Abort(500, "Error loading all users.")
      }
    }

    usersJson, err := users.Json()
    if err != nil {
      ctx.Abort(500, "Error getting users as json.")
    }

    return usersJson
  })

  web.Post("/api/users", func(ctx *web.Context) string {
    setJsonHeaders(ctx)
    body, err := ioutil.ReadAll(ctx.Request.Body)
    if err != nil {
      ctx.Abort(500, "Error reading request body.")
    }
    user, err := userStore.NewUserFromContainerJson(body)
    if err != nil {
      ctx.Abort(500, "Invalid JSON")
    }
    if err := user.Save(); err != nil {
      ctx.Abort(500, "Error saving user")
    }
    if err := userStore.SetMyUser(user); err != nil {
      ctx.Abort(500, "Error setting my user")
    }

    container := user.Container()
    userJson, err := container.Json()
    if err != nil {
      ctx.Abort(500, "Could not create container")
    }
    return userJson
  })

  web.Post("/api/posts", func(ctx *web.Context) string {
    setJsonHeaders(ctx)

    body, err := ioutil.ReadAll(ctx.Request.Body)
    if err != nil {
      ctx.Abort(500, "Error reading request body.")
    }

    user, err := userStore.GetMyUser()
    if err != nil {
      ctx.Abort(500, "Error reading my user when posting.")
    }

    post, err := user.NewPostFromContainerJson(body)
    if err != nil {
      ctx.Abort(500, "Invalid JSON")
    }

    if err := post.Save(); err != nil {
      ctx.Abort(500, "Error saving post")
    }

    for _, file := range post.Files {
      // Move the file from the tmp dir to the user's file dir.
      err := store.Move(path.Join(os.TempDir(), file.Hash), path.Join(user.FilePath(), file.Hash))
      if err != nil {
        ctx.Abort(500, fmt.Sprintf("Error opening temp file: %s", err))
      }
    }

    container := post.Container()

    postJson, err := container.Json()
    if err != nil {
      ctx.Abort(500, "Could not create container")
    }

    return postJson
  })

  web.Delete("/api/posts/(.*)", func(ctx *web.Context, id string) string {
    setJsonHeaders(ctx)
    user, err := userStore.GetMyUser()
    if err != nil {
      ctx.Abort(500, "Error loading user.")
    }

    posts, err := user.GetPosts()
    if err != nil {
      ctx.Abort(500, "Error loading posts.")
    }

    for _, post := range posts.Posts {
      if post.Id == id {
        err := post.Delete()
        if err != nil {
          ctx.Abort(500, "Error deleting post.")
        } else {
          return "OK"
        }
      }
    }

    return "OK"
  })

  web.Get("/api/get_folders", func(ctx *web.Context) string {
    setJsonHeaders(ctx)

    api := btsync.New("aaron", "lol", 1337, true)
    folders, err := api.GetFolders()
    if err != nil {
      ctx.Abort(500, fmt.Sprintf("get_folders: %s", err))
    }

    foldersJson, err := json.Marshal(folders)
    return string(foldersJson)
  })

  web.Post("/file/upload", func(ctx *web.Context) string {
    ctx.Request.ParseMultipartForm(10 * 1024 * 1024)

    // TODO(aaron): Find a way to stream this instead of buffering in memory.
    form := ctx.Request.MultipartForm
    fileHeader := form.File["file"][0]

    // Open the uploaded file and get its md5 hash.
    file, err := fileHeader.Open()
    if err != nil {
      ctx.Abort(500, fmt.Sprintf("Error reading file: %s", err))
    }
    defer file.Close()

    hash := Md5(file)
    file.Seek(0, os.SEEK_SET)

    // Create a temp file to save the uploaded file to.
    tmpFile, err := store.Create(os.TempDir(), hash)
    if err != nil {
      ctx.Abort(500, fmt.Sprintf("Error creating temp file: %s", err))
    }
    defer tmpFile.Close()

    // Copy the uploaded file to the temp file.
    _, err = io.Copy(tmpFile, file)
    if err != nil {
      ctx.Abort(500, fmt.Sprintf("Error creating temp file: %s", err))
    }

    // Return the md5 hash.
    setJsonHeaders(ctx)
    return fmt.Sprintf("{ \"hash\": \"%s\" }", hash)
  })

  web.Get("/", serveIndex)
  web.Get("/index.html", serveIndex)

  web.Run(config.Server.Listen)
}
