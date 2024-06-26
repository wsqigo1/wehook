package web

import (
	regexp "github.com/dlclark/regexp2"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/wsqigo/basic-go/webook/internal/domain"
	"github.com/wsqigo/basic-go/webook/internal/errs"
	"github.com/wsqigo/basic-go/webook/internal/service"
	jwt2 "github.com/wsqigo/basic-go/webook/internal/web/jwt"
	"github.com/wsqigo/basic-go/webook/pkg/ginx"
	"go.uber.org/zap"
	"net/http"
	"time"
)

const (
	emailRegexPattern    = `\w+([-+.]\w+)*@\w+([-.]\w+)*\.\w+([-.]\w+)*$`
	passwordRegexPattern = `^(?=.*[A-Za-z])(?=.*\d)(?=.*[$@$!%*#?&])[A-Za-z\d$@$!%*#?&]{8,}$`
	bizLogin             = "login"
)

// UserHandler 用户路由
type UserHandler struct {
	jwt2.Handler
	svc            service.UserService
	codeSvc        service.CodeService
	emailEexReg    *regexp.Regexp
	passwordRegExp *regexp.Regexp
}

func NewUserHandler(svc service.UserService,
	hdl jwt2.Handler, codeSvc service.CodeService) *UserHandler {
	return &UserHandler{
		svc:            svc,
		codeSvc:        codeSvc,
		emailEexReg:    regexp.MustCompile(emailRegexPattern, regexp.None),
		passwordRegExp: regexp.MustCompile(passwordRegexPattern, regexp.None),
		Handler:        hdl,
	}
}

func (h *UserHandler) RegisterRoutes(server *gin.Engine) {
	// 分组
	ug := server.Group("/users")
	ug.GET("/profile", ginx.WrapClaims(h.Profile))
	ug.POST("/signup", ginx.WrapBody(h.SignUp))
	//ug.POST("/login", h.Login)
	ug.POST("/login", ginx.WrapBody(h.LoginJWT))
	ug.POST("/logout", h.LogoutJWT)
	ug.POST("/edit", h.Edit)
	ug.POST("/refresh_token", h.RefreshToken)

	// 手机验证码登录相关功能
	ug.POST("/login_sms/code/send", ginx.WrapBody(h.SendSMSLoginCode))
	ug.POST("/login_sms", h.LoginSMS)
}

func (h *UserHandler) LoginSMS(ctx *gin.Context) {
	type Req struct {
		Phone string `json:"phone"`
		Code  string `json:"code"`
	}
	var req Req
	if err := ctx.Bind(&req); err != nil {
		return
	}

	ok, err := h.codeSvc.Verify(ctx, bizLogin, req.Phone, req.Code)
	if err != nil {
		ctx.JSON(http.StatusOK, ginx.Result{
			Code: 5,
			Msg:  "系统异常",
		})
		zap.L().Error("手机验证码验证失败",
			// 在生产环境绝对不能打
			// 开发环境你可以随便打
			zap.String("phone", req.Phone),
			zap.Error(err))
		return
	}
	if !ok {
		ctx.JSON(http.StatusOK, ginx.Result{
			Code: 4,
			Msg:  "验证码不对，请重新输入",
		})
		return
	}

	u, err := h.svc.FindOrCreate(ctx, req.Phone)
	if err != nil {
		ctx.JSON(http.StatusOK, ginx.Result{
			Code: 5,
			Msg:  "系统异常",
		})
		return
	}
	err = h.SetLoginToken(ctx, u.Id)
	if err != nil {
		ctx.JSON(http.StatusOK, ginx.Result{
			Code: 5,
			Msg:  "系统错误",
		})
		return
	}
	ctx.JSON(http.StatusOK, ginx.Result{
		Code: 2,
		Msg:  "登录成功",
	})
}

func (h *UserHandler) SendSMSLoginCode(ctx *gin.Context,
	req SendSMSCodeReq) (ginx.Result, error) {
	// 你这边可以校验 Req
	if req.Phone == "" {
		return ginx.Result{
			Code: 4,
			Msg:  "请输入手机号码",
		}, nil
	}

	err := h.codeSvc.Send(ctx, bizLogin, req.Phone)
	switch err {
	case nil:
		return ginx.Result{
			Code: 2,
			Msg:  "发送成功",
		}, nil
	case service.ErrCodeSendTooMany:
		// 事实上，防不住有人不知道怎么触发了
		// 少数这种错误，是可以接受的
		// 但是频繁出现，就代表有人在搞你的系统
		zap.L().Warn("频繁发送验证码")
		return ginx.Result{
			Code: 4,
			Msg:  "短信发送太频繁，请稍后再试",
		}, nil
	default:
		return ginx.Result{
			Code: 5,
			Msg:  "系统错误",
		}, err
		// 补日志的
	}
}

// SignUp 用户注册接口
func (h *UserHandler) SignUp(ctx *gin.Context, req SignUpReq) (ginx.Result, error) {
	isEmail, err := h.emailEexReg.MatchString(req.Email)
	if err != nil {
		return ginx.Result{
			Code: errs.UserInternalServerError,
			Msg:  "系统错误",
		}, err
	}
	if !isEmail {
		return ginx.Result{
			Code: errs.UserInvalidInput,
			Msg:  "非法邮箱格式",
		}, nil
	}

	if req.Password != req.ConfirmPassword {
		return ginx.Result{
			Code: errs.UserInvalidInput,
			Msg:  "两次输入的密码不相等",
		}, nil
	}

	isPassword, err := h.passwordRegExp.MatchString(req.ConfirmPassword)
	if err != nil {
		return ginx.Result{
			Code: errs.UserInternalServerError,
			Msg:  "系统错误",
		}, err
	}
	if !isPassword {
		return ginx.Result{
			Code: errs.UserInvalidInput,
			Msg:  "密码必须包含字母、数字、特殊字符",
		}, err
	}

	// 调用一下 svc 方法
	err = h.svc.SignUp(ctx.Request.Context(), domain.User{
		Email:    req.Email,
		Password: req.Password,
	})
	switch err {
	case nil:
		return ginx.Result{Msg: "注册成功"}, nil
	case service.ErrDuplicateEmail:
		return ginx.Result{
			Code: errs.UserDuplicateEmail,
			Msg:  "邮箱冲突",
		}, nil
	default:
		return ginx.Result{
			Code: errs.UserInternalServerError,
			Msg:  "系统错误",
		}, err
	}
}

func (h *UserHandler) Login(ctx *gin.Context) {
	type LoginReq struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	var req LoginReq
	if err := ctx.Bind(&req); err != nil {
		return
	}

	user, err := h.svc.Login(ctx, req.Email, req.Password)
	switch err {
	case nil:
		// 在这里登录成功了
		// 设置 session
		sess := sessions.Default(ctx)
		// 我可以随意设置值了
		// 你要放在 session 里面的值
		sess.Set("userId", user.Id)
		sess.Options(sessions.Options{
			//Secure: true,
			//HttpOnly: true,
			// 15 分钟
			MaxAge: 900,
		})
		err = sess.Save()
		if err != nil {
			ctx.String(http.StatusOK, "系统错误")
			return
		}

		ctx.String(http.StatusOK, "登录成功")
	case service.ErrInvalidUserOrPassword:
		ctx.String(http.StatusOK, "用户名或密码不对")
	default:
		ctx.String(http.StatusOK, "系统错误")
	}
}

func (h *UserHandler) LoginJWT(ctx *gin.Context, req LoginJWTReq) (ginx.Result, error) {
	user, err := h.svc.Login(ctx, req.Email, req.Password)
	switch err {
	case nil:
		err = h.SetLoginToken(ctx, user.Id)
		if err != nil {
			return ginx.Result{
				Code: errs.UserInternalServerError,
				Msg:  "系统错误",
			}, err
		}
		return ginx.Result{
			Msg: "OK",
		}, nil
	case service.ErrInvalidUserOrPassword:
		return ginx.Result{Msg: "用户名或者密码错误"}, nil
	default:
		return ginx.Result{Msg: "系统错误"}, err
	}
}

// Profile 用户详情
func (h *UserHandler) Profile(ctx *gin.Context, uc jwt2.UserClaims) (ginx.Result, error) {
	//us := ctx.MustGet("user").(jwt2.UserClaims)
	user, err := h.svc.FindById(ctx, uc.Uid)
	if err != nil {
		// 按照道理来说，这边 Id 对应的数据肯定存在，所以要是没找到，
		// 那说明是系统出了问题
		return ginx.Result{
			Code: errs.UserInternalServerError,
			Msg:  "系统错误",
		}, err
	}

	type User struct {
		Nickname string `json:"nickname"`
		Email    string `json:"email"`
		Phone    string `json:"phone"`
		AboutMe  string `json:"aboutMe"`
		Birthday string `json:"birthday"`
	}

	return ginx.Result{
		Data: User{
			Email:    user.Email,
			Nickname: user.Nickname,
			AboutMe:  user.AboutMe,
			Phone:    user.Phone,
			Birthday: user.Birthday.Format(time.DateOnly),
		},
	}, nil
}

func (h *UserHandler) Edit(ctx *gin.Context) {
	type EditReq struct {
		// 改邮箱，密码，或者能不能改手机号
		Nickname string `json:"nickname"`
		// YYYY-MM-DD
		Birthday string `json:"birthday"`
		AboutMe  string `json:"aboutMe"`
	}

	var req EditReq
	// Bind 方法会根据 Content-Type 来解析你的数据到 req 里面
	// 解析错了，就会直接写回一个 400 错误
	if err := ctx.Bind(&req); err != nil {
		return
	}

	uc, ok := ctx.MustGet("user").(jwt2.UserClaims)
	if !ok {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	// 用户输入不对
	birthday, err := time.Parse(time.DateOnly, req.Birthday)
	if err != nil {
		ctx.String(http.StatusOK, "生日格式不对")
		return
	}

	// 调用一下 svc 方法
	err = h.svc.UpdateNonSensitiveInfo(ctx, domain.User{
		Id:       uc.Uid,
		Nickname: req.Nickname,
		Birthday: birthday,
		AboutMe:  req.AboutMe,
	})
	if err != nil {
		ctx.String(http.StatusOK, "系统异常")
		return
	}

	ctx.String(http.StatusOK, "注册成功")
}

func (h *UserHandler) Logout(ctx *gin.Context) {
	sess := sessions.Default(ctx)
	// 我可以随便设置值了
	// 你要放在 session 里面的值
	sess.Options(sessions.Options{
		MaxAge: -1,
	})
	sess.Save()

	ctx.JSON(http.StatusOK, ginx.Result{Msg: "退出登录成功"})
}

func (h *UserHandler) LogoutJWT(ctx *gin.Context) {
	err := h.ClearToken(ctx)
	if err != nil {
		ctx.JSON(http.StatusOK, ginx.Result{
			Code: 5,
			Msg:  "系统错误",
		})
	}
	ctx.JSON(http.StatusOK, ginx.Result{Msg: "退出登录成功"})
}

func (h *UserHandler) RefreshToken(ctx *gin.Context) {
	// 约定，前端在 Authorization 里面带上这个 refresh_token
	tokenStr := h.ExtractToken(ctx)
	var rc jwt2.RefreshClaims
	token, err := jwt.ParseWithClaims(tokenStr, &rc, func(token *jwt.Token) (interface{}, error) {
		return jwt2.RCJWTKey, nil
	})

	// 这边要保持和登录校验一直的逻辑，即返回 401 响应
	if err != nil {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	if token == nil || !token.Valid {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	// 校验 ssid
	err = h.CheckSession(ctx, rc.Ssid)
	if err != nil {
		// 系统错误或者用户已经退出登录了
		// 这里也可以考虑说，如果在 Redis 已经崩溃的时候，
		// 就不要去校验是不是已经主动退出登录了。
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	err = h.SetJWTToken(ctx, rc.Uid, rc.Ssid)
	if err != nil {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	ctx.JSON(http.StatusOK, ginx.Result{
		Msg: "OK",
	})
}
