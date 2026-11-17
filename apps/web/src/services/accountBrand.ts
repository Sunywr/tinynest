const brands: Record<string, string> = {
  icbc: '工商银行 工行',
  abc: '农业银行 农行',
  boc: '中国银行 中行',
  ccb: '建设银行 建行',
  comm: '交通银行 交行',
  psbc: '邮政储蓄 邮储 邮政银行',
  cmb: '招商银行 招行',
  spdb: '浦发银行 浦发 浦东发展银行',
  citic: '中信银行 中信信用卡',
  ceb: '光大银行 光大信用卡',
  hxbank: '华夏银行 华夏信用卡',
  cmbc: '民生银行 民生信用卡',
  gdb: '广发银行 广发信用卡',
  cib: '兴业银行 兴业信用卡',
  spabank: '平安银行 平安信用卡',
  bjbank: '北京银行',
  shbank: '上海银行',
  jsbank: '江苏银行',
  nbbank: '宁波银行',
  njcb: '南京银行',
  hzcb: '杭州银行',
  czbank: '浙商银行',
  hsbank: '徽商银行',
  cscb: '长沙银行',
  gcb: '广州银行',
  cqbank: '重庆银行',
  cdcb: '成都银行',
  qdccb: '青岛银行',
  xabank: '西安银行',
  bhb: '河北银行',
  bohaib: '渤海银行',
  egbank: '恒丰银行',
  hsbc: '汇丰银行 汇丰',
  hkbea: '东亚银行',
  alipay: '支付宝 花呗 余额宝 借呗',
  wechat: '微信 零钱通 微粒贷',
  douyin: '抖音',
  meituan: '美团',
}

const aliases = Object.entries(brands).flatMap(([brand, names]) =>
  names.split(' ').map(name => ({ brand, name })),
).sort((a, b) => b.name.length - a.name.length)

export function accountBrand(...fields: (string | null | undefined)[]): string | undefined {
  for (const field of fields) {
    const name = field?.normalize('NFKC').toLowerCase().replace(/\\s+/g, '')
    if (!name) continue
    if (Object.hasOwn(brands, name)) return name
    const match = aliases.find(alias => name.includes(alias.name))
    if (match) return match.brand
  }
}
