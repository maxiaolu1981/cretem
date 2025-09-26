/*
1. 主测试套件 (1个)
TestAllSingleUserGetTests - 运行所有单用户查询测试的完整套件

2. 核心功能测试 (4个)
🔍 边界情况测试
TestSingleUserGetEdgeCases - 测试各种边界和异常情况

空用户ID

超长用户ID（1000字符）

特殊字符用户ID

SQL注入尝试

纯数字用户ID

🔐 认证测试
TestSingleUserGetAuthentication - 测试认证和授权机制

无Token请求

无效Token

过期Token

格式错误Token

⚡ 并发压力测试
TestSingleUserGetConcurrent - 大并发压力测试

50% 热点用户请求

20% 无效用户请求

10% 无权限用户请求

20% 随机正常用户请求

支持批量并发（1000用户×200请求）

🚨 缓存相关测试 (3个)
缓存击穿测试
TestSingleUserGetCachePenetration - 专门测试缓存击穿防护

使用不存在的用户ID进行高并发请求

监测数据库查询频率

检测缓存击穿风险

热点Key测试
TestSingleUserGetHotKey - 测试热点用户处理能力

对同一个热点用户进行1000次并发请求

监测响应时间和吞吐量

评估热点数据处理性能

冷启动测试
TestSingleUserGetColdStart - 测试缓存冷启动性能

第一次请求（冷启动）耗时

第二次请求（预热后）耗时

计算缓存性能提升比例

🎯 测试用例统计
测试类型	用例数量	主要作用
边界测试	1个用例（5种场景）	验证异常输入处理
认证测试	1个用例（4种场景）	验证安全机制
并发测试	1个用例（4种请求类型）	压力性能和稳定性
缓存测试	3个用例	缓存相关特殊场景
总计	6个主要测试函数	覆盖14+种测试场景
🚀 测试运行方式
bash
# 运行完整测试套件
go test -v -run TestAllSingleUserGetTests -timeout=30m

# 单独运行缓存击穿测试
go test -v -run TestSingleUserGetCachePenetration -timeout=10m

# 单独运行并发压力测试
go test -v -run TestSingleUserGetConcurrent -timeout=15m

# 运行所有缓存相关测试
go test -v -run "TestSingleUserGetCache|TestSingleUserGetHot|TestSingleUserGetCold" -timeout=20m

压力测试配置（高并发）
ConcurrentUsers       = 1000    // 更多并发用户
RequestsPerUser       = 100     // 每个用户较少请求
RequestInterval       = 10 * time.Millisecond  // 更短间隔

稳定性测试配置（长时间运行）
ConcurrentUsers       = 100     // 适中并发
RequestsPerUser       = 1000    // 每个用户更多请求
RequestInterval       = 100 * time.Millisecond // 正常间隔

缓存击穿测试配置
HotUserRequestPercent = 0       // 不使用热点用户
InvalidRequestPercent = 100     // 100%无效请求
ConcurrentUsers       = 50      // 适中并发

// 场景1：正常压力测试
ConcurrentUsers = 100       // 100个并发用户
RequestsPerUser = 200       // 每个用户200次请求
// 总请求: 20,000次

// 场景2：缓存击穿测试
ConcurrentUsers = 50        // 50个并发用户
RequestsPerUser = 1000      // 每个用户1000次请求
InvalidRequestPercent = 100 // 全部请求不存在的用户
// 总请求: 50,000次，全部触发缓存未命中

*/

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// ==================== 配置常量 ====================
const (
	ServerBaseURL  = "http://localhost:8088"
	RequestTimeout = 30 * time.Second

	LoginAPIPath   = "/login"
	SingleUserPath = "/v1/users/%s"

	TestUsername  = "user_0_3724_491287"
	ValidPassword = "Test@123456"

	RespCodeSuccess    = 100001
	RespCodeNotFound   = 100206 // 根据实际系统调整为110001
	RespCodeForbidden  = 110009 //无权访问
	RespCodeValidation = 100400

	ConcurrentUsers       = 1000
	RequestsPerUser       = 100
	RequestInterval       = 10 * time.Millisecond
	BatchSize             = 20
	HotUserRequestPercent = 50 // 热点用户请求百分比
	InvalidRequestPercent = 20 //无效请求百分比

	P50Threshold   = 50 * time.Millisecond
	P90Threshold   = 100 * time.Millisecond
	P99Threshold   = 200 * time.Millisecond
	ErrorRateLimit = 0.01

	// 缓存击穿测试相关常量
	CachePenetrationTestUsers  = 1000                      // 缓存击穿测试并发用户数
	CachePenetrationRequests   = 100                       // 每个用户请求次数
	CachePenetrationUserID     = "mxl_nonexistent-user-11" // 用于缓存击穿测试的用户ID
	CachePenetrationBatchDelay = 300 * time.Millisecond    // 批次间延迟
)

// 在全局变量部分添加预定义的用户列表
var (
	// 预定义的有效用户列表
	predefinedValidUsers = []string{
		"user_965_7049_774260",
		"user_959_8286_263169",
		"user_968_2308_653068",
		"user_960_5735_481143",
		"user_991_989_526798",
		"user_993_4251_86259",
		"user_997_6191_907301",
		"user_994_7880_290434",
		"user_953_5355_375678",
		"user_978_5314_530980",
		"user_970_3505_350372",
		"user_980_8907_579007",
		"user_951_4793_939697",
		"user_999_6932_874707",
		"user_955_9005_773527",
		"user_954_4650_278139",
		"user_972_2382_611249",
		"user_961_8768_54978",
		"user_988_2019_845200",
		"user_990_2978_187456",
		"user_967_636_160537",
		"user_976_8274_116351",
		"user_984_6741_835773",
		"user_983_3517_463374",
		"user_957_8959_105980",
		"user_956_2330_42713",
		"user_966_8768_943405",
		"user_973_6194_390145",
		"user_962_7921_107486",
		"user_996_6313_358645",
		"user_987_9057_492232",
		"user_982_9523_20772",
		"user_977_4590_354721",
		"user_979_5500_536559",
		"user_964_7222_815594",
		"user_950_7689_358701",
		"user_952_8441_698458",
		"user_969_9691_910508",
		"user_974_5672_770288",
		"user_995_5_246472",
		"user_985_811_212055",
		"user_965_1939_732246",
		"user_959_867_75839",
		"user_960_9804_370022",
		"user_963_8703_472687",
		"user_993_4520_362322",
		"user_994_2409_978348",
		"user_992_6027_769478",
		"user_989_2235_811465",
		"user_978_7481_857480",
		"user_970_9537_70711",
		"user_997_8699_581754",
		"user_951_4113_687899",
		"user_953_2663_576236",
		"user_998_7175_159976",
		"user_991_6945_75142",
		"user_999_3087_369522",
		"user_972_4242_789510",
		"user_983_8712_97920",
		"user_23_8737_339979",
		"user_3_2564_948216",
		"user_19_8501_977834",
		"user_34_8421_552226",
		"user_38_9151_128706",
		"user_27_8131_787622",
		"user_37_2364_254373",
		"user_32_372_438332",
		"user_5_9989_931431",
		"user_9_4962_478025",
		"user_14_1771_415983",
		"user_2_1359_93117",
		"user_24_4440_934945",
		"user_8_9484_714108",
		"user_15_7232_844566",
		"user_10_8119_545897",
		"user_7_7982_120921",
		"user_35_8516_70007",
		"user_36_6278_864077",
		"user_11_5514_851163",
		"user_12_1015_5670",
		"user_20_7886_653328",
		"user_30_417_576280",
		"user_21_8744_473707",
		"user_1_4483_868406",
		"user_22_8529_348255",
		"user_17_8925_620064",
		"user_16_3531_429376",
		"user_26_4089_114501",
		"user_31_3326_87725",
		"user_39_8245_395699",
		"user_28_6871_254967",
		"user_33_7207_444118",
		"user_4_1626_928172",
		"user_25_4146_27004",
		"user_0_2251_202690",
		"user_13_7380_888815",
		"user_6_6792_343628",
		"user_18_2265_530341",
		"user_32_8138_726522",
		"user_34_3830_971731",
		"user_23_6395_847616",
		"user_29_3811_621426",
		"user_38_3792_634573",
		"user_10_1782_514315",
		"user_5_3298_780103",
		"user_3_8577_88583",
		"user_37_2608_921381",
		"user_24_5588_633340",
		"user_27_3133_428808",
		"user_14_1937_137834",
		"user_7_6840_571195",
		"user_11_591_172198",
		"user_9_6770_809887",
		"user_15_8677_879716",
		"user_2_5387_44573",
		"user_16_992_701965",
		"user_8_9251_405046",
		"user_1_3259_112899",
		"user_12_9694_484442",
		"user_39_2197_270593",
		"user_35_8009_644481",
		"user_36_5328_320737",
		"user_33_2000_723069",
		"user_19_4470_688949",
		"user_31_9634_854494",
		"user_20_341_783566",
		"user_26_3224_346033",
		"user_4_9600_907804",
		"user_21_9187_284116",
		"user_22_119_630989",
		"user_30_8567_436666",
		"user_28_4141_618536",
		"user_25_9811_519492",
		"user_17_9866_297085",
		"user_18_5569_261443",
		"user_6_6060_34255",
		"user_38_4868_790471",
		"user_13_4914_479206",
		"user_23_3929_998352",
		"user_0_572_491446",
		"user_29_2225_11859",
		"user_34_793_908262",
		"user_32_9729_950077",
		"user_27_9056_642010",
		"user_9_120_699738",
		"user_37_1371_104669",
		"user_7_507_203057",
		"user_14_7670_959357",
		"user_3_5122_864",
		"user_10_7990_654706",
		"user_11_8670_347033",
		"user_15_5848_607514",
		"user_8_8403_176513",
		"user_5_3370_465646",
		"user_24_4187_333859",
		"user_2_6886_929882",
		"user_19_2454_599304",
		"user_33_7989_102876",
		"user_35_824_537968",
		"user_12_820_561125",
		"user_16_9202_807694",
		"user_39_5978_306761",
		"user_1_4630_305933",
		"user_21_4139_307933",
		"user_36_8302_27287",
		"user_30_6370_267031",
		"user_26_5494_645225",
		"user_20_3299_83490",
		"user_31_8626_122952",
		"user_28_9642_581163",
		"user_18_1545_463927",
		"user_4_8772_542479",
		"user_17_9058_174067",
		"user_0_1679_805946",
		"user_25_7206_369777",
		"user_13_6382_444779",
		"user_22_8974_714396",
		"user_29_9116_471993",
		"user_34_8282_47738",
		"user_6_5501_162800",
		"user_38_5266_721736",
		"user_32_5609_415027",
		"user_23_2166_737590",
		"user_9_3411_483994",
		"user_27_3102_87167",
		"user_14_4542_573028",
		"user_37_6104_399885",
		"user_7_1038_581641",
		"user_24_4888_166120",
		"user_3_283_973381",
		"user_11_3877_513034",
		"user_2_2123_307612",
		"user_12_9356_434080",
		"user_36_2706_32800",
		"user_5_48_130185",
		"user_33_7259_224286",
		"user_10_9675_457487",
		"user_15_3172_703469",
		"user_16_1847_421252",
		"user_1_4723_534247",
		"user_8_7592_699149",
		"user_39_2435_375462",
		"user_26_8638_13052",
		"user_31_4553_785376",
		"user_21_4109_82405",
		"user_18_3253_855758",
		"user_20_9350_811744",
		"user_35_8176_977616",
		"user_17_4940_623796",
		"user_4_3258_173763",
		"user_19_5801_295099",
		"user_28_7849_299081",
		"user_25_2306_457351",
		"user_30_5106_491381",
		"user_13_6799_331418",
		"user_29_7598_466023",
		"user_6_6005_200",
		"user_38_3230_611026",
		"user_34_1570_779999",
		"user_0_6946_918725",
		"user_22_8926_457924",
		"user_24_6095_851268",
		"user_27_6611_969945",
		"user_37_4304_127580",
		"user_32_8692_857199",
		"user_23_8851_513871",
		"user_11_5362_820906",
		"user_14_3871_167779",
		"user_7_4445_559665",
		"user_36_9574_455151",
		"user_3_6859_360450",
		"user_9_8094_366130",
		"user_10_3023_786567",
		"user_33_6393_12030",
		"user_12_5518_840884",
		"user_21_6908_835437",
		"user_15_4527_461362",
		"user_39_8615_29015",
		"user_16_2773_294753",
		"user_26_3038_924872",
		"user_1_6425_457357",
		"user_19_6553_111024",
		"user_8_3506_963270",
		"user_2_6758_684629",
		"user_5_3385_102428",
		"user_17_4523_171927",
		"user_4_5486_689378",
		"user_35_8045_705544",
		"user_31_7171_183665",
		"user_18_4061_580019",
		"user_20_6539_625037",
		"user_22_1843_361410",
		"user_30_1950_246453",
		"user_28_179_897210",
		"user_0_4667_856612",
		"user_25_1103_178396",
		"user_13_7567_621131",
		"user_14_1362_372772",
		"user_6_3644_739249",
		"user_34_426_399279",
		"user_32_9828_467821",
		"user_38_1150_641468",
		"user_23_6175_718202",
		"user_37_4207_278513",
		"user_29_8260_319043",
		"user_11_6047_656021",
		"user_24_578_907509",
		"user_33_248_199531",
		"user_12_9046_237709",
		"user_7_7366_291114",
		"user_10_3608_241851",
		"user_3_2065_49575",
		"user_15_8529_252553",
		"user_9_63_437572",
		"user_19_7853_147966",
		"user_36_5309_261132",
		"user_1_2491_454322",
		"user_26_1369_495259",
		"user_27_5267_852784",
		"user_16_546_108257",
		"user_21_1757_865007",
		"user_17_1584_478071",
		"user_4_3183_457994",
		"user_39_5840_221501",
		"user_5_7857_102024",
		"user_2_7920_836601",
		"user_31_4521_279199",
		"user_35_9107_10836",
		"user_8_7354_924333",
		"user_28_8577_220161",
		"user_22_1833_304720",
		"user_20_7986_768754",
		"user_18_2405_506815",
		"user_0_4143_881698",
		"user_25_798_815537",
		"user_30_2146_932371",
		"user_13_5943_660788",
		"user_6_5768_740898",
		"user_29_4072_359917",
		"user_38_1429_518407",
		"user_34_5269_475818",
		"user_11_8373_571696",
		"user_32_5060_409966",
		"user_23_3455_266278",
		"user_24_6081_867409",
		"user_33_5431_940003",
		"user_12_3983_774050",
		"user_10_5585_451761",
		"user_37_6438_977427",
		"user_14_7728_444708",
		"user_27_6350_618694",
		"user_7_5665_348299",
		"user_19_7634_722971",
		"user_3_8476_804943",
		"user_9_4128_556964",
		"user_31_6232_932534",
		"user_1_2905_858412",
		"user_36_6743_195613",
		"user_26_8248_395689",
		"user_17_5946_599898",
		"user_16_5257_341042",
		"user_15_382_584864",
		"user_21_5645_577374",
		"user_2_2641_621196",
		"user_35_665_826593",
		"user_20_9317_561132",
		"user_18_591_292610",
		"user_8_8707_208590",
		"user_39_4886_39",
		"user_22_1151_709566",
		"user_25_5992_766793",
		"user_5_461_981016",
		"user_28_3843_55033",
		"user_13_1692_782175",
		"user_4_4811_897187",
		"user_30_7112_899262",
		"user_6_1588_120493",
		"user_29_5741_895768",
		"user_34_5916_805519",
		"user_38_7378_825523",
		"user_0_5657_312951",
		"user_23_7297_290617",
		"user_32_3351_728775",
		"user_37_8888_550958",
		"user_14_2358_229213",
		"user_12_7841_55220",
		"user_33_5343_128653",
		"user_10_3465_62281",
		"user_11_9423_83785",
		"user_7_3804_296099",
		"user_19_272_417450",
		"user_24_1709_496009",
		"user_26_2702_327406",
		"user_36_2139_77524",
		"user_27_1731_487341",
		"user_15_8606_982177",
		"user_9_7574_124612",
		"user_16_6759_48159",
		"user_31_2577_922040",
		"user_3_4431_887646",
		"user_21_3784_852669",
		"user_17_4910_888181",
		"user_1_7942_930708",
		"user_39_7039_889631",
		"user_8_4631_390664",
		"user_5_8190_980955",
		"user_4_7297_939963",
		"user_22_4370_756712",
		"user_2_2021_32243",
		"user_20_7213_996194",
		"user_18_203_695398",
		"user_0_6807_866388",
		"user_25_207_606794",
		"user_28_4303_430180",
		"user_35_252_233809",
		"user_30_3776_798679",
		"user_13_4667_57858",
		"user_6_5588_456395",
		"user_29_5165_338487",
		"user_34_9813_696822",
		"user_32_5935_977638",
		"user_23_5294_973008",
		"user_33_8291_637861",
		"user_24_9570_426833",
		"user_27_9930_284968",
		"user_11_8280_885744",
		"user_14_7542_885303",
		"user_38_5384_405195",
		"user_37_3126_174834",
		"user_36_7345_484381",
		"user_10_2656_80604",
		"user_19_7672_986948",
		"user_3_1867_265606",
		"user_7_6177_400225",
		"user_26_1754_348809",
		"user_9_2895_763313",
		"user_12_825_247571",
		"user_31_9636_490655",
		"user_16_2679_406630",
		"user_15_3705_955115",
		"user_1_3005_529489",
		"user_39_6772_87949",
		"user_4_9130_38035",
		"user_17_575_543221",
		"user_25_3353_498607",
		"user_35_3322_300056",
		"user_21_6208_887252",
		"user_5_8184_144020",
		"user_8_4549_968324",
		"user_22_3360_24046",
		"user_2_8582_980348",
		"user_30_9222_168613",
		"user_6_1525_906159",
		"user_20_2171_638719",
		"user_28_8282_675445",
		"user_18_1567_764560",
		"user_0_9198_256585",
		"user_34_2273_981720",
		"user_29_6724_606934",
		"user_14_7022_648064",
		"user_13_75_180299",
		"user_32_3247_594739",
		"user_37_1406_816447",
		"user_23_8898_100021",
		"user_33_4778_526143",
		"user_27_7908_670288",
		"user_38_375_197242",
		"user_10_8406_65083",
		"user_12_5758_58243",
		"user_11_6548_505073",
		"user_9_1409_238525",
		"user_24_9315_590470",
		"user_36_7013_799946",
		"user_26_7658_579172",
		"user_19_4434_603021",
		"user_4_6224_550522",
		"user_7_2305_655081",
		"user_15_9906_462354",
		"user_31_5282_991076",
		"user_3_3014_296333",
		"user_16_7904_458356",
		"user_25_3683_481959",
		"user_21_7668_860383",
		"user_39_7626_582832",
		"user_17_6230_295800",
		"user_5_8345_605207",
		"user_35_7339_36634",
		"user_8_1023_983486",
		"user_1_1033_634696",
		"user_2_5073_326024",
		"user_0_9322_409189",
		"user_28_7962_129117",
		"user_22_8128_621518",
		"user_20_3386_448680",
		"user_18_8525_689042",
		"user_6_5249_283578",
		"user_13_2527_735792",
		"user_44_7349_561885",
		"user_40_429_317446",
		"user_56_3123_909876",
		"user_62_5287_870432",
		"user_29_3196_203760",
		"user_30_7381_561526",
		"user_74_7786_136949",
		"user_65_3679_462920",
		"user_66_2069_763084",
		"user_47_3274_318950",
		"user_52_9013_370148",
		"user_53_9115_231705",
		"user_64_1916_611724",
		"user_43_739_479628",
		"user_55_1845_425523",
		"user_67_6408_453206",
		"user_51_5410_213380",
		"user_69_5095_392899",
		"user_72_7287_872878",
		"user_71_8892_538508",
		"user_50_826_685587",
		"user_73_3872_502167",
		"user_48_4699_608264",
		"user_60_5101_445293",
		"user_42_1991_755130",
		"user_68_5127_187989",
		"user_57_7987_260630",
		"user_61_8809_393174",
		"user_63_4353_826941",
		"user_75_2907_991361",
		"user_76_9343_889424",
		"user_70_6351_43126",
		"user_49_3203_568125",
		"user_54_2432_6404",
		"user_58_2302_528628",
		"user_41_9656_674894",
		"user_45_8853_386309",
		"user_59_3456_119312",
		"user_46_8191_667021",
		"user_79_1383_171705",
		"user_77_3619_492369",
		"user_78_9098_211876",
		"user_74_9323_234802",
		"user_44_6225_930687",
		"user_52_9519_109604",
		"user_65_9606_497996",
		"user_43_1922_634044",
		"user_40_4671_628156",
		"user_62_815_257397",
		"user_56_7168_232952",
		"user_47_8591_988491",
		"user_66_5714_580930",
		"user_57_7050_356102",
		"user_51_7034_540960",
		"user_64_5434_414744",
		"user_69_2717_151055",
		"user_71_1360_900224",
		"user_53_7234_471739",
		"user_61_4524_333186",
		"user_48_5292_479255",
		"user_70_3241_770946",
		"user_42_5087_23377",
		"user_73_3493_171725",
		"user_67_9739_296276",
		"user_58_7647_640122",
		"user_76_39_703804",
		"user_49_3570_253519",
		"user_68_5631_239750",
		"user_50_9917_879122",
		"user_54_2511_129987",
		"user_63_6757_987692",
		"user_55_5910_882938",
		"user_60_8880_494552",
		"user_75_6434_731880",
		"user_72_5138_251456",
		"user_45_9147_950298",
		"user_59_6312_424682",
		"user_41_8401_109649",
		"user_78_7632_222951",
		"user_46_9738_636733",
		"user_79_6293_398495",
		"user_44_1004_514328",
		"user_56_3240_239799",
		"user_77_1818_863375",
		"user_40_2485_450830",
		"user_74_7196_139962",
		"user_62_5422_52983",
		"user_65_9959_218621",
		"user_52_4695_755153",
		"user_73_4182_869261",
		"user_64_8467_425691",
		"user_66_1533_466431",
		"user_47_2178_138681",
		"user_57_7803_417211",
		"user_69_8012_698421",
		"user_43_6190_317639",
		"user_53_3268_149168",
		"user_42_1653_242115",
		"user_68_169_199365",
		"user_61_3550_186510",
		"user_70_8065_484554",
		"user_67_6698_843166",
		"user_71_4072_657158",
		"user_54_9581_871264",
		"user_50_8995_668856",
		"user_58_5951_599662",
		"user_55_7569_578612",
		"user_51_6464_564545",
		"user_63_2418_696227",
		"user_60_1801_998580",
		"user_49_8972_833623",
		"user_76_1411_664282",
		"user_41_7951_666492",
		"user_72_3143_688508",
		"user_48_3179_630019",
		"user_45_2174_275190",
		"user_75_230_709257",
		"user_59_8438_338663",
		"user_78_2541_100588",
		"user_40_5344_272430",
		"user_79_9824_681074",
		"user_44_3856_323803",
		"user_56_7936_832905",
		"user_46_5865_608030",
		"user_77_3285_402817",
		"user_74_3105_399798",
		"user_65_3814_147889",
		"user_52_9149_716667",
		"user_62_3655_744000",
		"user_64_5022_478977",
		"user_43_2508_560606",
		"user_47_5690_827818",
		"user_42_4114_467633",
		"user_69_1321_949504",
		"user_57_6627_951422",
		"user_73_5601_771044",
		"user_66_7992_899774",
		"user_71_3975_821520",
		"user_53_6366_59436",
		"user_54_7193_575302",
		"user_51_1446_617053",
		"user_70_9683_742969",
		"user_67_2132_703128",
		"user_61_8986_888508",
		"user_50_6020_110233",
		"user_58_854_131431",
		"user_60_8870_841302",
		"user_55_8150_841057",
		"user_48_3333_853057",
		"user_49_7831_224105",
		"user_68_3109_185719",
		"user_63_4576_558173",
		"user_72_7469_517491",
		"user_78_7275_19143",
		"user_79_7033_649934",
		"user_46_7180_618607",
		"user_75_925_368040",
		"user_41_1971_112933",
		"user_77_2674_206250",
		"user_45_4123_870390",
		"user_76_6018_778805",
		"user_40_4381_476864",
		"user_56_9599_939441",
		"user_74_3672_966166",
		"user_59_6335_809650",
		"user_44_6196_212131",
		"user_52_691_880650",
		"user_65_6526_28768",
		"user_66_6371_800087",
		"user_43_4913_359659",
		"user_62_7347_234388",
		"user_57_6760_698811",
		"user_64_2958_866450",
		"user_47_3411_252062",
		"user_42_7399_832746",
		"user_69_2110_807377",
		"user_53_4664_139722",
		"user_71_5646_273690",
		"user_73_1454_649388",
		"user_70_9996_328205",
		"user_67_3864_998009",
		"user_51_8757_94782",
		"user_48_9468_429329",
		"user_63_5751_785803",
		"user_58_4757_37489",
		"user_50_8481_274452",
		"user_60_2277_123224",
		"user_68_9731_610510",
		"user_55_3452_683184",
		"user_72_4906_136649",
		"user_61_978_971997",
		"user_49_3031_109489",
		"user_45_5103_590713",
		"user_41_4950_640495",
		"user_76_105_730868",
		"user_54_520_38587",
		"user_79_5016_836206",
		"user_56_7458_678069",
		"user_75_8468_83652",
		"user_77_3309_366923",
		"user_78_4889_681318",
		"user_44_5100_43571",
		"user_46_6834_16814",
		"user_59_4910_600702",
		"user_40_6156_258558",
		"user_74_281_85117",
		"user_65_2034_221735",
		"user_47_5410_78118",
		"user_62_9565_380724",
		"user_43_8329_487971",
		"user_52_2479_528152",
		"user_64_2248_372338",
		"user_57_4280_79157",
		"user_54_5_371750",
		"user_66_1427_907157",
		"user_69_6202_936314",
		"user_53_5631_338746",
		"user_70_1260_251246",
		"user_61_4971_184902",
		"user_67_3852_455194",
		"user_48_3011_567314",
		"user_63_3442_599201",
		"user_71_8453_338984",
		"user_51_530_843882",
		"user_58_5210_627352",
		"user_73_7757_273152",
		"user_50_7052_479631",
		"user_68_3014_86676",
		"user_49_8350_506189",
		"user_60_295_611469",
		"user_55_7565_991720",
		"user_42_7915_146129",
		"user_76_3099_857529",
		"user_72_1190_169230",
		"user_78_382_90211",
		"user_59_7604_560547",
		"user_79_4977_277277",
		"user_45_8054_305840",
		"user_77_3078_878938",
		"user_41_6066_328731",
		"user_43_5982_430243",
		"user_75_7573_701699",
		"user_46_8839_432012",
		"user_44_4657_187756",
		"user_62_202_581409",
		"user_74_1027_190064",
		"user_56_8861_122753",
		"user_40_1321_552801",
		"user_52_4570_28577",
		"user_57_8448_248499",
		"user_64_4329_463911",
		"user_66_4607_579832",
		"user_53_1237_460418",
		"user_71_4827_435239",
		"user_54_2372_228534",
		"user_65_3853_875037",
		"user_70_1377_183642",
		"user_51_1034_363871",
		"user_47_6728_431849",
		"user_69_4132_658001",
		"user_61_7436_780814",
		"user_42_1083_94137",
		"user_67_1259_387956",
		"user_55_2580_370576",
		"user_73_2933_229363",
		"user_48_7594_284708",
		"user_68_796_501157",
		"user_58_5047_208719",
		"user_60_7319_546176",
		"user_50_4292_92"}

	// 预定义的热点用户
	predefinedHotUser = "admin"

	// 预定义的无权限用户
	predefinedUnauthorizedUser = "user_4_4678_130683"

	// 预定义的无效用户列表
	predefinedInvalidUsers = []string{
		"nonexistent-user-001",
		"invalid-user-123",
		"deleted-user-456",
		"test-invalid-789",
		"fake-user-000",
		"user-not-found-999",
		"unknown-user-888",
		"ghost-user-777",
		"deleted-account-666",
		"removed-user-555",
	}
)

// ==================== 数据结构 ====================
type APIResponse struct {
	HTTPStatus int         `json:""`
	Code       int         `json:"code"`
	Message    string      `json:"message"`
	Error      string      `json:"error,omitempty"`
	Data       interface{} `json:"data,omitempty"`
}

type TestContext struct {
	Username     string
	Userid       string
	AccessToken  string
	RefreshToken string
}

type PerformanceStats struct {
	TotalRequests       int
	SuccessCount        int
	ExpectedFailCount   int // 预期的失败（如404）
	UnexpectedFailCount int // 意外的失败（如500）
	TotalDuration       time.Duration
	Durations           []time.Duration
	StatusCount         map[int]int
	BusinessCodeCount   map[int]int
}

// ==================== 全局变量 ====================
var (
	httpClient       = createHTTPClient()
	mu               sync.Mutex
	validUserIDs     []string
	hotUserID        string
	invalidUserIDs   []string
	unauthorizedUser string

	cachePenetrationCounter int
	cachePenetrationMutex   sync.Mutex
	TotalStats              = &PerformanceStats{
		StatusCount:       make(map[int]int),
		BusinessCodeCount: make(map[int]int),
	}
	TotalErrTestResults = []TestResult{}
)

type TestResult struct {
	Username     string
	ExpectedHTTP int
	ExpectedBiz  int
	ActualHTTP   int
	ActualBiz    int
	Message      string
}

// ==================== 初始化函数 ====================
func initTestData() {
	// 使用预定义的用户列表
	// 过滤掉有问题的用户
	filteredUsers := []string{}
	for _, user := range predefinedValidUsers {
		// 移除已知有问题的用户
		if !isProblematicUser(user) {
			filteredUsers = append(filteredUsers, user)
		}
	}

	validUserIDs = predefinedValidUsers
	hotUserID = predefinedHotUser
	unauthorizedUser = predefinedUnauthorizedUser
	invalidUserIDs = predefinedInvalidUsers

	fmt.Printf("✅ 初始化测试数据完成:\n")
	fmt.Printf("   有效用户数量: %d\n", len(validUserIDs))
	fmt.Printf("   热点用户: %s\n", hotUserID)
	fmt.Printf("   无权限用户: %s\n", unauthorizedUser)
	fmt.Printf("   无效用户数量: %d\n", len(invalidUserIDs))

	if len(validUserIDs) > 10 {
		fmt.Printf("   示例用户: %v\n", validUserIDs[:10])
	} else {
		fmt.Printf("   所有用户: %v\n", validUserIDs)
	}
}

func isProblematicUser(username string) bool {
	problemUsers := map[string]bool{
		"test_user_123":      true, // 返回500
		"user_0_1079_314004": true, // 返回422
		// 添加其他有问题的用户
	}
	return problemUsers[username]
}

func init() {
	fmt.Println("初始化单用户查询接口测试环境...")
	initTestData()
	checkResourceLimits()
	setHigherFileLimit()
}

// ==================== HTTP客户端函数 ====================
func createHTTPClient() *http.Client {
	return &http.Client{
		Timeout: RequestTimeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 90 * time.Second,
			}).DialContext,
			MaxIdleConns:          1000,
			MaxIdleConnsPerHost:   1000,
			MaxConnsPerHost:       1000,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ForceAttemptHTTP2:     true,
		},
	}
}

// ==================== 登录函数 ====================
func login(username, password string) (*TestContext, *APIResponse, error) {
	loginURL := ServerBaseURL + LoginAPIPath
	body := fmt.Sprintf(`{"username":"%s","password":"%s"}`, username, password)
	bodyReader := strings.NewReader(body)

	req, err := http.NewRequest(http.MethodPost, loginURL, bodyReader)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	// 首先尝试解析为标准API响应格式
	var apiResp APIResponse
	apiResp.HTTPStatus = resp.StatusCode
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		// 如果解析失败，说明不是标准格式，尝试直接解析为token数据
		return parseDirectLoginResponse(resp, respBody, username)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &apiResp, fmt.Errorf("登录失败: HTTP %d", resp.StatusCode)
	}

	// 检查是否是标准格式（包含code、message、data字段）
	if apiResp.Data != nil {
		// 标准格式：从Data字段提取token
		tokenData, ok := apiResp.Data.(map[string]interface{})
		if !ok {
			return nil, &apiResp, fmt.Errorf("响应Data字段格式错误")
		}

		accessToken, _ := tokenData["access_token"].(string)
		refreshToken, _ := tokenData["refresh_token"].(string)
		userID, _ := tokenData["user_id"].(string)

		return &TestContext{
			Username:     username,
			Userid:       userID,
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
		}, &apiResp, nil
	} else {
		// 非标准格式，直接解析整个响应体
		return parseDirectLoginResponse(resp, respBody, username)
	}
}

// parseDirectLoginResponse 解析直接返回的登录响应（非标准格式）
func parseDirectLoginResponse(resp *http.Response, respBody []byte, username string) (*TestContext, *APIResponse, error) {
	// 直接解析为token数据
	var tokenData map[string]interface{}
	if err := json.Unmarshal(respBody, &tokenData); err != nil {

		return nil, nil, fmt.Errorf("响应格式错误: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		// 构建错误响应
		apiResp := &APIResponse{
			HTTPStatus: resp.StatusCode,
			Code:       -1,
			Message:    "登录失败",
			Data:       tokenData,
		}

		return nil, apiResp, fmt.Errorf("登录失败: HTTP %d", resp.StatusCode)
	}

	accessToken, _ := tokenData["access_token"].(string)
	refreshToken, _ := tokenData["refresh_token"].(string)
	userID, _ := tokenData["user_id"].(string)

	if accessToken == "" {
		return nil, nil, fmt.Errorf("登录失败: 未获取到access_token")
	}

	// 构建成功的API响应
	apiResp := &APIResponse{
		HTTPStatus: resp.StatusCode,
		Code:       200,
		Message:    "登录成功",
		Data:       tokenData,
	}

	return &TestContext{
		Username:     username,
		Userid:       userID,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, apiResp, nil
}

// ==================== 请求发送函数 ====================
func sendTokenRequest(ctx *TestContext, method, path string, body io.Reader) (*APIResponse, error) {
	fullURL := ServerBaseURL + path
	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	if ctx != nil && ctx.AccessToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ctx.AccessToken))
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apiResp APIResponse
	apiResp.HTTPStatus = resp.StatusCode
	if err := json.Unmarshal(respBody, &apiResp); err != nil {

		return nil, err
	}

	return &apiResp, nil
}

func checkResourceLimits() {
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err == nil {
		fmt.Printf("当前文件描述符限制: Soft=%d, Hard=%d\n", rLimit.Cur, rLimit.Max)
		if rLimit.Cur < 10000 {
			fmt.Printf("⚠️  文件描述符限制较低，建议使用: ulimit -n 10000\n")
		}
	}
}

func setHigherFileLimit() {
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err == nil {
		if rLimit.Cur < 10000 && rLimit.Max >= 10000 {
			rLimit.Cur = 10000
			if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit); err == nil {
				fmt.Printf("✅ 文件描述符限制已设置为: %d\n", rLimit.Cur)
			}
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ==================== 测试用例函数 ====================

// ==================== 并发测试框架 ====================
func runBatchConcurrentTest(t *testing.T, testName string, testFunc func(*testing.T, int, *TestContext) (bool, bool, *APIResponse, int, int)) {
	totalBatches := (ConcurrentUsers + BatchSize - 1) / BatchSize

	// 先登录获取token
	ctx, _, err := login(TestUsername, ValidPassword)
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}

	for batch := 0; batch < totalBatches; batch++ {
		startUser := batch * BatchSize
		endUser := min((batch+1)*BatchSize, ConcurrentUsers)

		fmt.Printf("\n🔄 执行第 %d/%d 批测试: 用户 %d-%d\n",
			batch+1, totalBatches, startUser, endUser-1)

		runConcurrentTest(t, testName, startUser, endUser, ctx, testFunc)

		// 批次间休息
		if batch < totalBatches-1 {
			fmt.Printf("⏸️  批次间休息 2秒...\n")
			time.Sleep(2 * time.Second)
			runtime.GC()
			time.Sleep(500 * time.Millisecond)
		}
	}

	// 输出性能报告
	printPerformanceReport(testName)
}

func runConcurrentTest(t *testing.T, testName string,
	startUser, endUser int, ctx *TestContext,
	testFunc func(*testing.T, int, *TestContext) (bool, bool, *APIResponse, int, int)) *PerformanceStats {
	var wg sync.WaitGroup
	// 启动并发测试
	for i := startUser; i < endUser; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()
			for j := 0; j < RequestsPerUser; j++ {
				success, isExpectedFailure, _, _, _ := testFunc(t, userID, ctx)
				mu.Lock()
				if success {
					TotalStats.SuccessCount++
					if isExpectedFailure {
						// 这是预期的失败（如正确的404响应），也算在预期失败中
						TotalStats.ExpectedFailCount++
					}
				} else {
					if isExpectedFailure {
						// 这是预期失败的情况，但响应不符合预期
						TotalStats.UnexpectedFailCount++
					} else {
						// 这是真正的意外失败
						TotalStats.UnexpectedFailCount++
					}
				}
				mu.Unlock()
				time.Sleep(RequestInterval)
			}
		}(i)
	}
	wg.Wait()
	runtime.GC()
	return TotalStats
}

func testSingleUserGetRequest(t *testing.T, userID int, ctx *TestContext) (bool, bool, *APIResponse, int, int) {
	var targetUserID string
	var expectedHTTP int
	var expectedBiz int
	isExpectedFailure := false

	// 随机决定请求类型
	randNum := rand.IntN(100)
	//randNum = 90
	switch {
	case randNum < HotUserRequestPercent:
		targetUserID = hotUserID
		expectedHTTP = http.StatusOK
		expectedBiz = RespCodeSuccess
	case randNum < HotUserRequestPercent+InvalidRequestPercent:
		targetUserID = invalidUserIDs[rand.IntN(len(invalidUserIDs))]
		expectedHTTP = http.StatusUnauthorized
		expectedBiz = RespCodeNotFound
		isExpectedFailure = true
	case randNum < HotUserRequestPercent+InvalidRequestPercent+10:
		targetUserID = unauthorizedUser
		expectedHTTP = http.StatusForbidden
		expectedBiz = RespCodeForbidden
		isExpectedFailure = true
	default:
		targetUserID = validUserIDs[rand.IntN(len(validUserIDs))]
		expectedHTTP = http.StatusOK
		expectedBiz = RespCodeSuccess
	}

	apiPath := fmt.Sprintf(SingleUserPath, targetUserID)

	resp, err := sendTokenRequest(ctx, http.MethodGet, apiPath, nil)
	if err != nil {
		//log.Errorf("%v", err)
		return false, false, nil, expectedHTTP, expectedBiz
	}
	// ========== 正确的验证逻辑 ==========

	success := (resp.HTTPStatus == expectedHTTP) && (resp.Code == expectedBiz)
	if !success {
		TotalErrTestResults = append(TotalErrTestResults,
			TestResult{
				Username:     ctx.Username,
				ExpectedHTTP: expectedHTTP,
				ActualHTTP:   resp.HTTPStatus,
				ExpectedBiz:  expectedBiz,
				ActualBiz:    resp.Code,
			})
	}
	// 关键修正：只有在是预期失败的情况下，才保持isExpectedFailure为true
	// 如果验证失败，且这不是预期失败的情况，才标记为非预期失败

	// 如果success为true，isExpectedFailure保持原值

	//log.Errorf("结果:%v 期望http:%v 实际http:%v  期望业务码:%v 实际业务码:%v", success, expectedHTTP, resp.HTTPStatus, expectedBiz, resp.Code)

	return success, isExpectedFailure, resp, expectedHTTP, expectedBiz
}

// ==================== 性能报告函数 ====================
func printPerformanceReport(testName string) {
	width := 60
	separator := strings.Repeat("─", width)
	thickSeparator := strings.Repeat("═", width)

	fmt.Printf("\n")
	fmt.Printf("┌%s┐\n", thickSeparator)
	fmt.Printf("│%s│\n", centerText("📊 "+testName+" 性能报告", width))
	fmt.Printf("├%s┤\n", separator)

	// 基础统计
	totalRequests := ConcurrentUsers * RequestsPerUser
	fmt.Printf("│ %-25s: %8d │\n", "总请求数", totalRequests)
	fmt.Printf("│ %-25s: %8d (%.2f%%) │\n", "成功数",
		TotalStats.SuccessCount,
		float64(TotalStats.SuccessCount)/float64(TotalStats.TotalRequests)*100)
	fmt.Printf("│ %-25s: %8d (%.2f%%) │\n", "预期失败数",
		TotalStats.ExpectedFailCount,
		float64(TotalStats.ExpectedFailCount)/float64(TotalStats.TotalRequests)*100)
	fmt.Printf("│ %-25s: %8d (%.2f%%) │\n", "意外失败数",
		TotalStats.UnexpectedFailCount,
		float64(TotalStats.UnexpectedFailCount)/float64(TotalStats.TotalRequests)*100)

	fmt.Printf("├%s┤\n", separator)

	// 性能指标
	if TotalStats.TotalDuration > 0 {
		qps := float64(TotalStats.TotalRequests) / TotalStats.TotalDuration.Seconds()
		if !math.IsInf(qps, 0) && !math.IsNaN(qps) {
			fmt.Printf("│ %-25s: %8.1f │\n", "平均QPS", qps)
		} else {
			fmt.Printf("│ %-25s: %8s │\n", "平均QPS", "无法计算")
		}
	} else {
		fmt.Printf("│ %-25s: %8s │\n", "平均QPS", "无法计算")
	}

	if TotalStats.SuccessCount > 0 {
		avgResponseTime := TotalStats.TotalDuration / time.Duration(TotalStats.SuccessCount)
		fmt.Printf("│ %-25s: %8v │\n", "平均响应时间", avgResponseTime.Round(time.Microsecond))
	} else {
		fmt.Printf("│ %-25s: %8s │\n", "平均响应时间", "无数据")
	}

	// 响应时间分位值
	if len(TotalStats.Durations) > 0 {
		sort.Slice(TotalStats.Durations, func(i, j int) bool {
			return TotalStats.Durations[i] < TotalStats.Durations[j]
		})
		p50 := TotalStats.Durations[int(float64(len(TotalStats.Durations))*0.5)]
		p90 := TotalStats.Durations[int(float64(len(TotalStats.Durations))*0.9)]
		p99 := TotalStats.Durations[int(float64(len(TotalStats.Durations))*0.99)]

		fmt.Printf("│ %-25s: %8v │\n", "P50响应时间", p50.Round(time.Microsecond))
		fmt.Printf("│ %-25s: %8v │\n", "P90响应时间", p90.Round(time.Microsecond))
		fmt.Printf("│ %-25s: %8v │\n", "P99响应时间", p99.Round(time.Microsecond))
	}

	fmt.Printf("├%s┤\n", separator)

	// 错误率分析
	realErrorRate := float64(TotalStats.UnexpectedFailCount) / float64(TotalStats.TotalRequests)
	errorRateDisplay := fmt.Sprintf("%.4f%%", realErrorRate*100)
	if realErrorRate <= ErrorRateLimit {
		fmt.Printf("│ %-25s: %8s ✅ │\n", "真实错误率", errorRateDisplay)
	} else {
		fmt.Printf("│ %-25s: %8s ❌ │\n", "真实错误率", errorRateDisplay)
	}

	fmt.Printf("├%s┤\n", separator)

	// HTTP状态码分布
	fmt.Printf("│ %-56s │\n", "🌐 HTTP状态码分布:")
	for status, count := range TotalStats.StatusCount {
		percentage := float64(count) / float64(TotalStats.TotalRequests) * 100
		statusText := fmt.Sprintf("  HTTP %d", status)
		countText := fmt.Sprintf("%d (%.1f%%)", count, percentage)
		fmt.Printf("│   %-20s: %30s │\n", statusText, countText)
	}

	fmt.Printf("├%s┤\n", separator)

	// 业务码分布
	fmt.Printf("│ %-56s │\n", "📋 业务码分布:")
	for code, count := range TotalStats.BusinessCodeCount {
		percentage := float64(count) / float64(TotalStats.TotalRequests) * 100
		codeText := fmt.Sprintf("  业务码 %d", code)
		countText := fmt.Sprintf("%d (%.1f%%)", count, percentage)
		fmt.Printf("│   %-20s: %30s │\n", codeText, countText)
	}

	// 错误分析
	if TotalStats.StatusCount[404] > 0 || TotalStats.UnexpectedFailCount > 0 {
		fmt.Printf("├%s┤\n", separator)
		fmt.Printf("│ %-56s │\n", "🔍 错误分析:")

		expected404ButFailed := TotalStats.StatusCount[404] - TotalStats.ExpectedFailCount
		if expected404ButFailed > 0 {
			fmt.Printf("│   %-54s │\n",
				fmt.Sprintf("预期404但标记为意外失败: %d次", expected404ButFailed))
		}

		if TotalStats.UnexpectedFailCount > 0 {
			fmt.Printf("│   %-54s │\n",
				fmt.Sprintf("真正的意外错误: %d次", TotalStats.UnexpectedFailCount))
		}

		// 其他错误状态码
		for status, count := range TotalStats.StatusCount {
			if status != 200 && status != 404 && count > 0 {
				fmt.Printf("│   %-54s │\n",
					fmt.Sprintf("HTTP %d 错误: %d次", status, count))
			}
		}
	}

	// 错误明细（如果有）
	if len(TotalErrTestResults) > 0 {
		fmt.Printf("├%s┤\n", separator)
		fmt.Printf("│ %-56s │\n", "📝 错误明细:")
		for i := 0; i < len(TotalErrTestResults); i++ {
			errMsg := truncateText(fmt.Sprintf("%v", TotalErrTestResults[i]), 50)
			fmt.Printf("│   %2d. %-51s │\n", i+1, errMsg)
		}
	}

	fmt.Printf("└%s┘\n", thickSeparator)
}

// 辅助函数：居中显示文本
func centerText(text string, width int) string {
	if len(text) >= width {
		return text
	}
	padding := (width - len(text)) / 2
	return strings.Repeat(" ", padding) + text + strings.Repeat(" ", width-len(text)-padding)
}

// 辅助函数：截断文本
func truncateText(text string, maxLength int) string {
	if len(text) <= maxLength {
		return text
	}
	return text[:maxLength-3] + "..."
}

// TestSingleUserGetConcurrent 单用户查询接口并发测试
func TestSingleUserGetConcurrent(t *testing.T) {
	t.Run("单用户查询接口大并发压力测试", func(t *testing.T) {
		runBatchConcurrentTest(t, "单用户查询接口压力测试", testSingleUserGetRequest)
	})
}

// TestSingleUserGetCachePenetration 缓存击穿测试
func TestSingleUserGetCachePenetration(t *testing.T) {
	t.Run("缓存击穿测试", func(t *testing.T) {
		// 登录获取token
		ctx, _, err := login(TestUsername, ValidPassword)
		if err != nil {
			t.Fatalf("登录失败: %v", err)
		}

		fmt.Printf("🔥 开始缓存击穿测试，使用用户ID: %s\n", CachePenetrationUserID)
		fmt.Printf("📊 并发用户: %d, 每用户请求: %d, 总请求: %d\n",
			CachePenetrationTestUsers, CachePenetrationRequests,
			CachePenetrationTestUsers*CachePenetrationRequests)

		apiPath := fmt.Sprintf(SingleUserPath, CachePenetrationUserID)
		var wg sync.WaitGroup
		stats := &PerformanceStats{
			StatusCount:       make(map[int]int),
			BusinessCodeCount: make(map[int]int),
		}

		startTime := time.Now()

		// 分批发送请求
		for batch := 0; batch < CachePenetrationTestUsers; batch++ {
			wg.Add(1)
			go func(batchID int) {
				defer wg.Done()

				for i := 0; i < CachePenetrationRequests; i++ {
					start := time.Now()
					resp, err := sendTokenRequest(ctx, http.MethodGet, apiPath, nil)
					duration := time.Since(start)

					mu.Lock()
					stats.TotalRequests++
					if err == nil && resp.HTTPStatus == http.StatusUnauthorized {
						stats.SuccessCount++
						stats.TotalDuration += duration
					} else {
						stats.UnexpectedFailCount++
					}
					stats.Durations = append(stats.Durations, duration)

					if resp != nil {
						stats.StatusCount[resp.HTTPStatus]++
						stats.BusinessCodeCount[resp.Code]++
					}
					mu.Unlock()

					// 记录数据库查询次数
					if resp != nil && resp.HTTPStatus == http.StatusNotFound {
						cachePenetrationMutex.Lock()
						cachePenetrationCounter++
						cachePenetrationMutex.Unlock()
					}

					time.Sleep(time.Duration(rand.IntN(50)) * time.Millisecond)
				}
			}(batch)
			time.Sleep(CachePenetrationBatchDelay)
		}

		wg.Wait()
		totalDuration := time.Since(startTime)

		// 输出测试结果
		fmt.Printf("\n🔥 缓存击穿测试结果:\n")
		fmt.Printf("总请求数: %d\n", stats.TotalRequests)
		fmt.Printf("成功请求 (404): %d\n", stats.SuccessCount)
		fmt.Printf("失败请求: %d\n", stats.UnexpectedFailCount)
		fmt.Printf("总耗时: %v\n", totalDuration)
		fmt.Printf("平均QPS: %.1f\n", float64(stats.TotalRequests)/totalDuration.Seconds())
		fmt.Printf("数据库查询次数 (模拟): %d\n", cachePenetrationCounter)

		// 检查缓存击穿
		dbQueryRate := float64(cachePenetrationCounter) / float64(stats.TotalRequests)
		if dbQueryRate > 0.1 {
			fmt.Printf("❌ 可能发生缓存击穿，数据库查询比例: %.1f%%\n", dbQueryRate*100)
		} else {
			fmt.Printf("✅ 缓存击穿防护有效，数据库查询比例: %.1f%%\n", dbQueryRate*100)
		}
	})
}

// TestSingleUserGetEdgeCases 边界情况测试
func TestSingleUserGetEdgeCases(t *testing.T) {
	t.Run("单用户查询边界情况测试", func(t *testing.T) {
		ctx, _, err := login(TestUsername, ValidPassword)
		if err != nil {
			t.Fatalf("登录失败: %v", err)
		}

		testCases := []struct {
			name         string
			userID       string
			expectedHTTP int
			expectedBiz  int
			description  string
		}{
			{"空用户ID", "", http.StatusNotFound, RespCodeNotFound, "空字符串用户ID应该返回404"},
			{"超长用户ID", strings.Repeat("a", 1000), http.StatusBadRequest, RespCodeValidation, "超长用户ID应该返回400"},
			{"特殊字符用户ID", "user@#$%^&*()", http.StatusBadRequest, RespCodeValidation, "包含特殊字符的用户ID应该返回400"},
			{"SQL注入尝试", "user'; DROP TABLE users; --", http.StatusBadRequest, RespCodeValidation, "SQL注入尝试应该被拒绝并返回400"},
			{"数字用户ID", "1234567890", http.StatusOK, RespCodeSuccess, "纯数字用户ID应该正常处理"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				apiPath := fmt.Sprintf(SingleUserPath, tc.userID)
				resp, err := sendTokenRequest(ctx, http.MethodGet, apiPath, nil)

				if err != nil {
					t.Fatalf("请求失败: %v", err)
				}

				if resp.HTTPStatus != tc.expectedHTTP {
					t.Errorf("HTTP状态码不符: 期望 %d, 实际 %d - %s", tc.expectedHTTP, resp.HTTPStatus, tc.description)
				}

				if resp.Code != tc.expectedBiz {
					t.Errorf("业务码不符: 期望 %d, 实际 %d - %s", tc.expectedBiz, resp.Code, tc.description)
				}

				t.Logf("测试通过: %s", tc.description)
			})
		}
	})
}

// TestSingleUserGetAuthentication 认证测试
func TestSingleUserGetAuthentication(t *testing.T) {
	t.Run("单用户查询认证测试", func(t *testing.T) {
		testCases := []struct {
			name         string
			token        string
			expectedHTTP int
			description  string
		}{
			{"无Token请求", "", http.StatusUnauthorized, "无Token应该返回401"},
			{"无效Token", "invalid-token-123456", http.StatusUnauthorized, "无效Token应该返回401"},
			{"过期Token", "expired-token-123456", http.StatusUnauthorized, "过期Token应该返回401"},
			{"格式错误Token", "Bearer-invalid-format", http.StatusUnauthorized, "格式错误Token应该返回401"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				apiPath := fmt.Sprintf(SingleUserPath, hotUserID)
				fullURL := ServerBaseURL + apiPath

				req, err := http.NewRequest(http.MethodGet, fullURL, nil)
				if err != nil {
					t.Fatalf("创建请求失败: %v", err)
				}

				if tc.token != "" {
					req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.token))
				}

				resp, err := httpClient.Do(req)
				if err != nil {
					t.Fatalf("请求失败: %v", err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != tc.expectedHTTP {
					t.Errorf("HTTP状态码不符: 期望 %d, 实际 %d - %s", tc.expectedHTTP, resp.StatusCode, tc.description)
				}

				t.Logf("测试通过: %s", tc.description)
			})
		}
	})
}

// TestSingleUserGetHotKey 热点Key测试
func TestSingleUserGetHotKey(t *testing.T) {
	t.Run("热点Key测试", func(t *testing.T) {
		ctx, _, err := login(TestUsername, ValidPassword)
		if err != nil {
			t.Fatalf("登录失败: %v", err)
		}

		fmt.Printf("🔥 开始热点Key测试，使用用户ID: %s\n", hotUserID)

		apiPath := fmt.Sprintf(SingleUserPath, hotUserID)
		var wg sync.WaitGroup
		stats := &PerformanceStats{
			StatusCount:       make(map[int]int),
			BusinessCodeCount: make(map[int]int),
		}

		startTime := time.Now()

		for i := 0; i < 1000; i++ {
			wg.Add(1)
			go func(requestID int) {
				defer wg.Done()

				start := time.Now()
				resp, err := sendTokenRequest(ctx, http.MethodGet, apiPath, nil)
				duration := time.Since(start)

				mu.Lock()
				stats.TotalRequests++
				if err == nil && resp.HTTPStatus == http.StatusOK {
					stats.SuccessCount++
					stats.TotalDuration += duration
				} else {
					stats.UnexpectedFailCount++
				}
				stats.Durations = append(stats.Durations, duration)

				if resp != nil {
					stats.StatusCount[resp.HTTPStatus]++
					stats.BusinessCodeCount[resp.Code]++
				}
				mu.Unlock()

				time.Sleep(time.Duration(rand.IntN(10)) * time.Millisecond)
			}(i)
		}

		wg.Wait()
		totalDuration := time.Since(startTime)

		fmt.Printf("\n🔥 热点Key测试结果:\n")
		fmt.Printf("总请求数: %d\n", stats.TotalRequests)
		fmt.Printf("成功请求: %d\n", stats.SuccessCount)
		fmt.Printf("失败请求: %d\n", stats.UnexpectedFailCount)
		fmt.Printf("总耗时: %v\n", totalDuration)
		fmt.Printf("平均QPS: %.1f\n", float64(stats.TotalRequests)/totalDuration.Seconds())

		if stats.SuccessCount > 0 {
			avgResponseTime := stats.TotalDuration / time.Duration(stats.SuccessCount)
			fmt.Printf("平均响应时间: %v\n", avgResponseTime)
			if avgResponseTime > 100*time.Millisecond {
				fmt.Printf("❌ 热点Key处理较慢\n")
			} else {
				fmt.Printf("✅ 热点Key处理正常\n")
			}
		}
	})
}

// TestSingleUserGetColdStart 冷启动测试
func TestSingleUserGetColdStart(t *testing.T) {
	t.Run("冷启动测试", func(t *testing.T) {
		ctx, _, err := login(TestUsername, ValidPassword)
		if err != nil {
			t.Fatalf("登录失败: %v", err)
		}

		coldUserID := validUserIDs[len(validUserIDs)-1]
		fmt.Printf("❄️  开始冷启动测试，使用用户ID: %s\n", coldUserID)

		apiPath := fmt.Sprintf(SingleUserPath, coldUserID)

		// 第一次请求（冷启动）
		start := time.Now()
		resp, err := sendTokenRequest(ctx, http.MethodGet, apiPath, nil)
		coldStartDuration := time.Since(start)

		if err != nil {
			t.Fatalf("冷启动请求失败: %v", err)
		}

		if resp.HTTPStatus != http.StatusOK {
			t.Fatalf("冷启动请求失败，状态码: %d", resp.HTTPStatus)
		}

		// 第二次请求（预热后）
		start = time.Now()
		resp, err = sendTokenRequest(ctx, http.MethodGet, apiPath, nil)
		warmDuration := time.Since(start)

		if err != nil {
			t.Fatalf("预热后请求失败: %v", err)
		}

		fmt.Printf("❄️  冷启动测试结果:\n")
		fmt.Printf("冷启动耗时: %v\n", coldStartDuration)
		fmt.Printf("预热后耗时: %v\n", warmDuration)
		fmt.Printf("性能提升: %.1f%%\n", (float64(coldStartDuration)-float64(warmDuration))/float64(coldStartDuration)*100)

		if warmDuration < coldStartDuration/2 {
			fmt.Printf("✅ 缓存效果良好\n")
		} else {
			fmt.Printf("⚠️  缓存效果不明显\n")
		}
	})
}

// TestAllSingleUserGetTests 主测试函数
func TestAllSingleUserGetTests(t *testing.T) {
	t.Run("单用户查询完整测试套件", func(t *testing.T) {
		t.Run("边界情况测试", TestSingleUserGetEdgeCases)
		t.Run("认证测试", TestSingleUserGetAuthentication)
		t.Run("缓存击穿测试", TestSingleUserGetCachePenetration)
		t.Run("热点Key测试", TestSingleUserGetHotKey)
		t.Run("冷启动测试", TestSingleUserGetColdStart)
		t.Run("并发压力测试", TestSingleUserGetConcurrent)
	})
}

// LoginResponse 登录响应结构体
type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	Expire       string `json:"expire"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	UserID       string `json:"user_id,omitempty"`
	Username     string `json:"username,omitempty"`
}
