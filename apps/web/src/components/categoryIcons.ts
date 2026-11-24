import {
  Apple, ArrowLeftRight, Baby, Backpack, Banknote, Beef, Bike, BookOpen, BriefcaseBusiness, Building2,
  Bus, CakeSlice, Car, Cat, Cherry, CircleDollarSign, ClipboardList, Cloud, Clover,
  Coffee, Cookie, CreditCard, Croissant, CupSoda, Dog, Dumbbell, Film, Fish, Flower2, Fuel,
  Gamepad2, Gift, GraduationCap, Hammer, Heart, HeartPulse, Hospital, House, IceCreamCone,
  Landmark, WashingMachine, LibraryBig, Lightbulb, Mail, MapPin, Milk, Music, Package,
  Palette, PawPrint, Phone, PiggyBank, Pill, Pizza, Plane, Rabbit, ReceiptText, Ribbon,
  Sandwich, Shield, Shirt, ShoppingBag, ShoppingBasket, ShoppingCart, Smartphone, Smile,
  Sofa, Sprout, Star, Stethoscope, Store, Tent, ToyBrick, TrainFront, TrendingUp,
  Trophy, Umbrella, Utensils, Wallet, Wifi, Zap,
} from 'lucide-vue-next'

type IconEntry = [value: string, label: string, icon: typeof Star]

export const categoryIconGroups: { name: string; entries: IconEntry[] }[] = [
  { name: '记账常用', entries: [
    ['snack', '零食', Cookie], ['toys', '玩具', ToyBrick], ['games', '游戏', Gamepad2],
    ['fun', '娱乐', Film], ['housing', '住房', House], ['car', '汽车', Car],
    ['redpacket', '红包', Gift], ['meal', '正餐', Utensils], ['fitness', '健身', Dumbbell],
  ] },
  { name: '餐饮美食', entries: [
    ['🍙', '饭团', Sandwich], ['meal', '正餐', Utensils], ['🍓', '水果', Cherry],
    ['apple', '苹果', Apple], ['🧁', '蛋糕', CakeSlice], ['🧋', '奶茶', CupSoda],
    ['🍵', '咖啡茶饮', Coffee], ['🥐', '可颂', Croissant], ['pizza', '披萨', Pizza],
    ['meat', '肉类', Beef], ['seafood', '海鲜', Fish], ['milk', '牛奶', Milk],
    ['ice-cream', '冰淇淋', IceCreamCone],
  ] },
  { name: '购物居家', entries: [
    ['🏠', '小家', House], ['🧺', '篮子', ShoppingBasket], ['🛍️', '购物', ShoppingBag],
    ['groceries', '超市', ShoppingCart], ['shop', '商店', Store], ['clothing', '服饰', Shirt],
    ['furniture', '家具', Sofa], ['utilities', '水电', Zap], ['internet', '网络', Wifi],
    ['lighting', '灯具', Lightbulb], ['repair', '维修', Hammer], ['phone', '话费', Phone],
    ['parcel', '快递', Package], ['laundry', '洗衣', WashingMachine],
  ] },
  { name: '交通旅行', entries: [
    ['🚲', '骑行', Bike], ['🚃', '出行', TrainFront], ['🚗', '汽车', Car],
    ['bus', '公交', Bus], ['fuel', '加油', Fuel], ['flight', '飞机', Plane],
    ['🧳', '旅行', Backpack], ['location', '地点', MapPin], ['camping', '露营', Tent],
    ['hotel', '住宿', Building2],
  ] },
  { name: '工作学习', entries: [
    ['💼', '工作', BriefcaseBusiness], ['📚', '书本', LibraryBig], ['reading', '阅读', BookOpen],
    ['study', '教育培训', GraduationCap], ['office', '办公', ClipboardList],
    ['device', '数码设备', Smartphone], ['💌', '信封', Mail],
  ] },
  { name: '娱乐运动', entries: [
    ['🎮', '游戏', Gamepad2], ['🎨', '画板', Palette], ['🏸', '运动', Trophy],
    ['fitness', '健身', Dumbbell], ['movies', '电影', Film], ['music', '音乐', Music],
    ['🧸', '玩偶', ToyBrick], ['🎀', '蝴蝶结', Ribbon], ['🎁', '礼物', Gift],
  ] },
  { name: '健康家庭', entries: [
    ['💊', '药品', Pill], ['doctor', '看诊', Stethoscope], ['hospital', '医院', Hospital],
    ['health', '健康', HeartPulse], ['insurance', '保险', Shield], ['rainy-day', '应急', Umbrella],
    ['baby', '育儿', Baby], ['family', '家人', Heart], ['pets', '宠物', PawPrint],
    ['🐰', '兔兔', Rabbit], ['🐱', '猫咪', Cat], ['dog', '狗狗', Dog],
  ] },
  { name: '收支金融', entries: [
    ['🪙', '金币', CircleDollarSign], ['💰', '钱袋', Wallet], ['salary', '工资', Banknote],
    ['savings', '储蓄', PiggyBank], ['investment', '投资', TrendingUp], ['bank', '银行', Landmark],
    ['card', '信用卡', CreditCard], ['receipt', '账单报销', ReceiptText],
  ] },
  { name: '自然与其他', entries: [
    ['🌷', '郁金香', Flower2], ['🌱', '幼苗', Sprout], ['☁️', '云朵', Cloud],
    ['🍀', '幸运草', Clover], ['⭐', '星星', Star], ['🐻', '小熊', Smile],
  ] },
]

const normalize = (value: string) => value.replace(/\uFE0F/g, '')
const icons = new Map(categoryIconGroups.flatMap(group => group.entries.map(
  ([value, , icon]) => [normalize(value), icon] as const,
)))

export function categoryIcon(value: string) {
  const key = normalize(value)
  if (key === '🔁') return ArrowLeftRight
  if (key === '📝') return ReceiptText
  return icons.get(key) ?? Star
}
