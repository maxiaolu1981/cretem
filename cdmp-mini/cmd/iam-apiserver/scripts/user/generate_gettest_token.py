import jwt
import time

# 核心配置（与服务端完全一致）
JWT_SECRET = "dfVpOK8LZeJLZHYmHdb1VdyRrACKpqoo"  # 服务端密钥
APIServerIssuer = "https://github.com/maxiaolu1981/cretem"
APIServerAudience = "https://github.com/maxiaolu1981/cretem"
EXP = int(time.time()) + 3600  # 有效期1小时

# Payload 配置：identity 与 sub 必须一致（均为 gettest-user101）
payload = {
    "aud": APIServerAudience,
    "exp": EXP,
    "identity": "gettest-user101",  # ✅ 修正：与 sub 一致
    "iss": APIServerIssuer,
    "origin_iat": EXP,
    "sub": "gettest-user104"        # ✅ 正确：目标用户名
}

# 生成 Token
token = jwt.encode(payload, JWT_SECRET, algorithm="HS256")
print(f"Bearer {token}")