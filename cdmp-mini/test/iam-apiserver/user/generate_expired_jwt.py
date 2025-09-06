import jwt
import time

# 配置（必须与服务端一致）
JWT_SECRET = "dfVpOK8LZeJLZHYmHdb1VdyRrACKpqoo"  # 服务端jwt.key
EXP = int(time.time()) - 3600  # 过期时间：当前时间-1小时

# Payload字段（严格匹配服务端预期，与有效Token一致）
payload = {
    "aud": "https://github.com/maxiaolu1981/cretem",
    "exp": EXP,
    "identity": "admin",
    "iss": "https://github.com/maxiaolu1981/cretem",
    "origin_iat": EXP,  # 与exp一致（或用有效Token中的原始值）
    "sub": "gettest-user101"
}

# 生成Token（使用PyJWT库，与服务端gin-jwt逻辑一致）
expired_token = jwt.encode(
    payload,
    JWT_SECRET,
    algorithm="HS256"
)

# 输出最终Token（带Bearer前缀）
print(f"Bearer {expired_token}")