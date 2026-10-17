// Every shipped marketplace definition (db/marketplaces/*.json) against its synthetic fixture
// (testdata/fixtures/marketplaces/<key>.html). The fixtures reproduce the layout each definition
// assumes with invented data; passing here does not mean a definition works on live pages.
import { existsSync, readFileSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'
import { domainMatches } from '../src/page'
import { parseRuleDocument, ruleFieldsOf } from '../src/rules/schema'
import { BUILTIN_ADAPTERS, type Payload } from '../src/types'
import { analyze, marketplaceDefinitions, rulesAdapter } from './helpers'

const ROOT = path.resolve(path.dirname(new URL(import.meta.url).pathname), '../../..')

interface Expect {
  title: string
  price: string
  currency: string
  images: string[]
  variants?: { count: number; first: Partial<Payload['variants'][number]> }
  specs?: Record<string, string>
  extra?: (p: Payload, metadata: Record<string, { value: string }>) => void
}

const EXPECTED: Record<string, Expect> = {
  aliexpress: {
    title: 'Folding LED Desk Lamp, 3 Colour Modes',
    price: '18.4',
    currency: 'USD',
    images: ['https://ae01.alicdn.example/kf/Slamp1.jpg', 'https://ae01.alicdn.example/kf/Slamp2.jpg', 'https://ae01.alicdn.example/kf/Slamp3.jpg'],
    variants: { count: 3, first: { sku: '12000031001', options: { Color: 'Black', 'Ships From': 'CHINA' }, price_amount: '18.4', currency: 'USD', availability: 'in stock', image_url: 'https://ae01.alicdn.example/kf/Sblack.jpg' } },
    specs: { 'Brand Name': 'LUMOFOLD', 'Power Source': 'USB' },
    extra: (p, m) => {
      expect(p.brand).toBe('LUMOFOLD')
      expect(p.model).toBe('LF-D3')
      expect(p.category).toBe('Lights & Lighting > Indoor Lighting > Desk Lamps')
      expect(p.supplier_name).toBe('Lumofold Lighting Store')
      expect(p.supplier_url).toBe('https://www.aliexpress.com/store/1102233')
      expect(p.availability).toBe('in stock')
      expect(p.variants[1]).toMatchObject({ options: { Color: 'Black', 'Ships From': 'United States' }, availability: 'out of stock' })
      expect(m['list_price']?.value).toBe('32 USD')
      expect(m['shipping']?.value).toBe('Free shipping, delivery in 7-12 days')
      expect(m['rating_count']?.value).toBe('312')
    },
  },
  'alibaba-com': {
    title: 'Custom Logo Bamboo Fibre Lunch Box Set',
    price: '2.85',
    currency: 'USD',
    images: ['https://s.alicdn.example/@sc04/kf/Hbox1.jpg', 'https://s.alicdn.example/@sc04/kf/Hbox2.jpg'],
    variants: { count: 3, first: { sku: '10001', options: { Color: 'Sage Green', Capacity: '800 ml' }, price_amount: '2.85', currency: 'USD', image_url: 'https://s.alicdn.example/@sc04/kf/Hgreen.jpg' } },
    specs: { Material: 'Bamboo fibre', 'Place of Origin': 'Zhejiang, China' },
    extra: (p) => {
      expect(p.brand).toBe('Greenleaf')
      expect(p.model).toBe('BF-LB2')
      expect(p.supplier_name).toBe('Ningbo Greenleaf Housewares Co., Ltd.')
      expect(p.supplier_url).toBe('https://greenleaf.en.alibaba.example/')
      expect(p.variants[1]).toMatchObject({ options: { Color: 'Sage Green', Capacity: '1200 ml' }, price_amount: '3.1' })
    },
  },
  'alibaba-1688': {
    title: '加厚纯棉儿童睡衣套装 秋冬款',
    price: '12.50',
    currency: 'CNY',
    images: ['https://cbu01.alicdn.example/img/ibank/O1CN01pjA1.jpg', 'https://cbu01.alicdn.example/img/ibank/O1CN01pjA2.jpg'],
    variants: { count: 3, first: { sku: '4455001', options: { 颜色: '灰色', 尺码: '110cm' }, price_amount: '12.50', currency: 'CNY', availability: 'in stock', image_url: 'https://cbu01.alicdn.example/img/ibank/O1CN01grey.jpg' } },
    specs: { 材质: '纯棉', 适用季节: '秋季, 冬季' },
    extra: (p) => {
      expect(p.supplier_name).toBe('义乌市恒达日用品有限公司')
      expect(p.variants[1]).toMatchObject({ options: { 颜色: '灰色', 尺码: '120cm' }, availability: 'out of stock' })
    },
  },
  taobao: {
    title: '复古陶瓷马克杯 大容量咖啡杯',
    price: '39',
    currency: 'CNY',
    images: ['https://img.alicdn.example/imgextra/i1/O1CN01mug1.jpg', 'https://img.alicdn.example/imgextra/i2/O1CN01mug2.jpg'],
    variants: { count: 3, first: { sku: '5001', options: { 颜色分类: '米白', 容量: '350ml' }, price_amount: '39', currency: 'CNY', availability: 'in stock', image_url: 'https://img.alicdn.example/imgextra/i3/O1CN01white.jpg' } },
    specs: { 品牌: '青瓷', 材质: '陶瓷' },
    extra: (p, m) => {
      expect(p.brand).toBe('青瓷')
      expect(p.supplier_name).toBe('青瓷小铺')
      expect(p.supplier_url).toBe('https://shop123456.taobao.com/')
      expect(p.variants[1]).toMatchObject({ options: { 颜色分类: '米白', 容量: '500ml' }, price_amount: '45', availability: 'out of stock' })
      expect(m['shipping']?.value).toBe('快递: 免运费')
    },
  },
  tmall: {
    title: '静音无线鼠标 可充电 办公笔记本通用',
    price: '59.90',
    currency: 'CNY',
    images: ['https://img.alicdn.example/imgextra/i1/2200001/O1CN01mouse1.jpg', 'https://img.alicdn.example/imgextra/i2/2200001/O1CN01mouse2.png'],
    variants: { count: 2, first: { sku: '880001', options: { 颜色分类: '雅黑' }, price_amount: '59.90', currency: 'CNY', availability: 'in stock' } },
    specs: { 品牌: '静点', 连接方式: '无线2.4G' },
    extra: (p) => {
      expect(p.brand).toBe('静点')
      expect(p.model).toBe('JD-M3')
      expect(p.supplier_name).toBe('静点数码旗舰店')
      expect(p.variants[1]).toMatchObject({ options: { 颜色分类: '月白' }, price_amount: '64.90' })
    },
  },
  jd: {
    title: '轻薄笔记本电脑 14英寸 16GB 512GB 星空灰',
    price: '4999.00',
    currency: 'CNY',
    images: ['https://img14.360buyimg.com/n0/jfs/t1/111/22/333/abc.jpg', 'https://img14.360buyimg.com/n0/jfs/t1/111/22/334/def.jpg'],
    variants: { count: 2, first: { sku: '100012345678', options: { 颜色: '星空灰', 版本: '16GB+512GB' }, title: '星空灰 / 16GB+512GB' } },
    specs: { 品牌: '星河', 型号: 'S14', 屏幕尺寸: '14英寸' },
    extra: (p) => {
      expect(p.sku).toBe('100012345678')
      expect(p.brand).toBe('星河')
      expect(p.model).toBe('S14')
      expect(p.category).toBe('电脑、办公 > 电脑整机 > 笔记本')
      expect(p.supplier_name).toBe('星河电脑京东自营旗舰店')
      expect(p.supplier_url).toBe('https://mall.jd.example/index-1000000904.html')
    },
  },
  dhgate: {
    title: 'Wireless Sport Earbuds IPX5 with Charging Case',
    price: '8.1',
    currency: 'USD',
    images: ['https://img4.dhgate.example/webp/m/0x0/f3/albu/g1/earbuds-1.jpg', 'https://img4.dhgate.example/webp/m/0x0/f3/albu/g1/earbuds-2.jpg'],
    variants: { count: 2, first: { sku: '9001', options: { Color: 'Black', Plug: 'USB-C' }, price_amount: '8.1', currency: 'USD', availability: 'in stock', image_url: 'https://img4.dhgate.example/webp/m/0x0/f3/albu/g1/black.jpg' } },
    specs: { 'Bluetooth Version': '5.3', 'Battery Life': '6 hours' },
    extra: (p, m) => {
      expect(p.sku).toBe('887766554')
      expect(p.category).toBe('Electronics > Earphones')
      expect(p.supplier_name).toBe('Shenzhen Audio Gear Store')
      expect(p.supplier_url).toBe('https://www.dhgate.com/store/21889900')
      expect(m['shipping']?.value).toBe('Ships within 3 business days')
    },
  },
  cjdropshipping: {
    title: 'Magnetic Phone Holder for Car Vent',
    price: '3.42',
    currency: 'USD',
    images: ['https://cf.cjdropshipping.example/quick/product/holder-1.jpg', 'https://cf.cjdropshipping.example/quick/product/holder-2.jpg'],
    variants: { count: 2, first: { title: 'Black', sku: 'CJYD1234567-Black', options: { Variant: 'Black' }, price_amount: '3.42', currency: 'USD', availability: 'in stock', image_url: 'https://cf.cjdropshipping.example/quick/product/holder-black.jpg' } },
    specs: { Material: 'ABS + aluminium', 'Packing weight': '110 g' },
    extra: (p) => {
      expect(p.sku).toBe('CJYD1234567')
      expect(p.description).toBe('Strong N52 magnets hold phones up to 6.9 inch.\nFits most air vents.')
      expect(p.category).toBe('Phone Accessories > Car Holders')
      expect(p.variants[1]).toMatchObject({ sku: 'CJYD1234567-Silver', price_amount: '3.58', availability: 'out of stock' })
    },
  },
  banggood: {
    title: 'Voltra 65W GaN USB-C Charger 3 Ports',
    price: '24.99',
    currency: 'USD',
    images: ['https://imgaz.staticbg.example/thumb/large/oaupload/banggood/images/AB/CD/charger-1.jpg', 'https://imgaz.staticbg.example/thumb/large/oaupload/banggood/images/AB/CD/charger-2.jpg'],
    specs: { Brand: 'Voltra', Output: '65W max', Ports: '2 x USB-C, 1 x USB-A' },
    extra: (p, m) => {
      expect(p.brand).toBe('Voltra')
      expect(p.sku).toBe('1987654')
      expect(p.category).toBe('Electronics > Chargers')
      expect(m['list_price']?.value).toBe('US$39.99')
      expect(m['shipping']?.value).toMatch(/^Free shipping to United States/)
      expect(m['rating_count']?.value).toBe('508')
    },
  },
  geekbuying: {
    title: 'Arvo X3 Robot Vacuum Cleaner 4000Pa',
    price: '219.99',
    currency: 'EUR',
    images: ['https://img.gkbcdn.example/p/2026-01-05/arvo-x3-1.jpg', 'https://img.gkbcdn.example/p/2026-01-05/arvo-x3-2.jpg'],
    specs: { Brand: 'Arvo', Suction: '4000Pa', Battery: '5200mAh' },
    extra: (p, m) => {
      expect(p.sku).toBe('552233')
      expect(p.category).toBe('Home Appliances > Robot Vacuums')
      expect(m['list_price']?.value).toBe('€299.99')
      expect(m['shipping']?.value).toBe('Ships from: EU Warehouse, delivery in 3-7 days')
    },
  },
  tomtop: {
    title: 'Portable Solar Power Bank 20000mAh with Flashlight',
    price: '45.99',
    currency: 'USD',
    images: ['https://img.tomtop.example/images/goods/solar-bank-1.jpg', 'https://img.tomtop.example/images/goods/solar-bank-2.jpg'],
    specs: { Capacity: '20000mAh', Input: 'USB-C 5V/2A', Weight: '420 g' },
    extra: (p, m) => {
      expect(p.brand).toBe('Solvio')
      expect(p.sku).toBe('P0123456')
      expect(p.category).toBe('Electronics > Power Banks')
      expect(p.warnings.join(' ')).not.toMatch(/ambiguous/)
      expect(m['list_price']?.value).toBe('$59.99')
    },
  },
  saleyee: {
    title: 'Ergonomic Mesh Office Chair with Lumbar Support',
    price: '89.50',
    currency: 'USD',
    images: ['https://img.saleyee.example/goods/chair-1.jpg', 'https://img.saleyee.example/goods/chair-2.jpg'],
    specs: { Material: 'Mesh, nylon base', 'Max Load': '150 kg', 'Package Size': '66 x 30 x 58 cm' },
    extra: (p, m) => {
      expect(p.sku).toBe('SY10233')
      expect(p.description).toMatch(/^Breathable mesh office chair/)
      expect(m['shipping']?.value).toBe('US warehouse: ships in 1-2 business days')
    },
  },
  temu: {
    title: 'Silicone Kitchen Utensil Set, 12 Pieces',
    price: '15.99',
    currency: 'USD',
    images: ['https://img.kwcdn.example/product/fancy/utensil-1.jpg', 'https://img.kwcdn.example/product/fancy/utensil-2.jpg'],
    variants: { count: 2, first: { sku: '17592186044416', title: 'Grey', options: { Color: 'Grey' }, price_amount: '15.99', currency: 'USD', availability: 'in stock', image_url: 'https://img.kwcdn.example/product/fancy/utensil-grey.jpg' } },
    specs: { Material: 'Silicone, Wood', Pieces: '12' },
    extra: (p, m) => {
      expect(p.category).toBe('Home & Kitchen > Kitchen Utensils & Gadgets')
      expect(p.supplier_name).toBe('Home Joy Mall')
      expect(p.variants[1]).toMatchObject({ options: { Color: 'Khaki' }, price_amount: '16.99', availability: 'out of stock' })
      expect(m['list_price']?.value).toBe('28.99 USD')
      expect(m['rating_count']?.value).toBe('2041')
    },
  },
  shein: {
    title: 'Ribbed Knit Crew Neck Sweater',
    price: '18.49',
    currency: 'USD',
    images: [
      'https://img.ltwebstatic.example/images3_pi/2026/01/01/sweater-main.jpg',
      'https://img.ltwebstatic.example/images3_pi/2026/01/01/sweater-2.jpg',
      'https://img.ltwebstatic.example/images3_pi/2026/01/01/sweater-3.jpg',
    ],
    variants: { count: 3, first: { sku: 'I12345abcS', options: { Size: 'S' }, price_amount: '18.49', currency: 'USD', availability: 'in stock' } },
    specs: { Color: 'Beige', Composition: '100% Acrylic' },
    extra: (p, m) => {
      expect(p.sku).toBe('sz2401010123456789')
      expect(p.category).toBe('Women Sweaters')
      expect(p.warnings.join(' ')).not.toMatch(/ambiguous/)
      expect(p.variants[1]).toMatchObject({ options: { Size: 'M' }, availability: 'out of stock' })
      expect(m['list_price']?.value).toBe('$29.99')
    },
  },
  rosegal: {
    title: 'Floral Print Tiered Maxi Dress',
    price: '27.99',
    currency: 'USD',
    images: [
      'https://gloimg.rglcdn.example/rosegal/pdm-product-pic/Clothing/2026/01/01/goods-img/dress-1.jpg',
      'https://gloimg.rglcdn.example/rosegal/pdm-product-pic/Clothing/2026/01/01/goods-img/dress-2.jpg',
    ],
    specs: { Material: 'Polyester', Silhouette: 'A-Line', 'Sleeve Length': 'Sleeveless' },
    extra: (p, m) => {
      expect(p.sku).toBe('471122334')
      expect(p.category).toBe('Dresses > Maxi Dresses')
      expect(m['list_price']?.value).toBe('45.99 USD')
    },
  },
  wish: {
    title: 'Stainless Steel Insulated Water Bottle 750 ml',
    price: '14',
    currency: 'USD',
    images: ['https://canary.contestimg.example/64a1b2c3d4e5f6a7b8c9d0e1-large.jpg', 'https://canary.contestimg.example/64a1b2c3d4e5f6a7b8c9d0e1-1-large.jpg'],
    variants: { count: 2, first: { sku: 'PH-750-OB', title: 'Ocean Blue / 750 ml', options: { Color: 'Ocean Blue', Size: '750 ml' }, price_amount: '14', currency: 'USD', availability: 'in stock' } },
    extra: (p, m) => {
      expect(p.description).toBe('Double-wall vacuum insulation keeps drinks cold for 24 hours.')
      expect(p.supplier_name).toBe('Peak Hydration')
      expect(p.supplier_url).toBe('https://www.wish.com/merchant/5f1e2d3c4b5a69788796a5b4')
      expect(p.variants[1]).toMatchObject({ availability: 'out of stock', price_amount: '15' })
      expect(m['list_price']?.value).toBe('29 USD')
      expect(m['rating_count']?.value).toBe('980')
    },
  },
  walmart: {
    title: 'Cast Iron Skillet, 12 inch, Pre-Seasoned',
    price: '24.97',
    currency: 'USD',
    images: ['https://i5.walmartimages.example/seo/skillet-12-1.jpeg', 'https://i5.walmartimages.example/seo/skillet-12-2.jpeg'],
    variants: { count: 2, first: { sku: '5099887766', options: { Color: 'Black', Size: '12 inch' }, price_amount: '24.97', currency: 'USD', availability: 'in_stock', image_url: 'https://i5.walmartimages.example/seo/skillet-black.jpeg' } },
    specs: { Material: 'Cast Iron', Diameter: '12 in' },
    extra: (p, m) => {
      expect(p.brand).toBe('Hearthline')
      expect(p.model).toBe('HL-CI12')
      expect(p.gtin).toBe('012345678905')
      expect(p.sku).toBe('5099887766')
      expect(p.description).toBe('Pre-seasoned cast iron skillet for stovetop, oven and campfire.')
      expect(p.category).toBe('Home > Kitchen & Dining > Cookware')
      expect(p.availability).toBe('in_stock')
      expect(p.supplier_name).toBe('Walmart.com')
      expect(p.variants[1]).toMatchObject({ options: { Color: 'Red', Size: '10 inch' }, price_amount: '21.97', availability: 'out_of_stock' })
      expect(m['list_price']?.value).toBe('34.97 USD')
      expect(m['shipping']?.value).toBe('Free shipping, arrives in 2 days')
    },
  },
  etsy: {
    title: 'Hand-thrown Stoneware Planter with Drainage',
    price: '38.00',
    currency: 'USD',
    images: ['https://i.etsystatic.example/12345/r/il/abc111/1111/il_fullxfull.1111.jpg', 'https://i.etsystatic.example/12345/r/il/abc222/2222/il_fullxfull.2222.jpg'],
    specs: { Materials: 'Stoneware, glaze', Height: '12 centimetres' },
    extra: (p, m) => {
      expect(p.brand).toBe('ClayAndFern')
      expect(p.sku).toBe('1500123456')
      expect(p.category).toBe('Home & Living > Home Décor > Planters')
      expect(p.supplier_name).toBe('ClayAndFern')
      expect(p.supplier_url).toBe('https://www.etsy.com/shop/ClayAndFern')
      expect(p.variants).toEqual([])
      expect(m['rating_count']?.value).toBe('212')
    },
  },
  kohls: {
    title: "Men's Hearth & Field Flannel Button-Down Shirt",
    price: '22.99',
    currency: 'USD',
    images: ['https://media.kohlsimg.example/is/image/kohls/6123456_Red_Plaid?wid=1500', 'https://media.kohlsimg.example/is/image/kohls/6123456_Navy_Plaid?wid=1500'],
    variants: { count: 2, first: { sku: '92011', options: { Color: 'Red Plaid', Size: 'M' }, price_amount: '22.99', currency: 'USD', availability: 'in stock' } },
    specs: { Fabric: 'Cotton', Care: 'Machine wash' },
    extra: (p, m) => {
      expect(p.brand).toBe('Hearth & Field')
      expect(p.sku).toBe('6123456')
      expect(p.description).toBe('Soft brushed cotton flannel\nButton-down collar')
      expect(p.category).toBe('Men > Shirts > Button-Down Shirts')
      expect(m['list_price']?.value).toBe('40 USD')
    },
  },
  costco: {
    title: 'Northpeak 6-Person Instant Cabin Tent',
    price: '229.99',
    currency: 'USD',
    images: [
      'https://images.costco-static.example/ImageDelivery/imageService?profileId=12026540&itemId=1712345-847&recipeName=680',
      'https://images.costco-static.example/ImageDelivery/imageService?profileId=12026540&itemId=1712345-894&recipeName=680',
    ],
    specs: { Brand: 'Northpeak', Capacity: '6 Person', 'Floor Dimensions': '3.0 x 2.7 m' },
    extra: (p, m) => {
      expect(p.sku).toBe('1712345')
      expect(p.brand).toBe('Northpeak')
      expect(p.description).toBe('Sets up in about 60 seconds with pre-attached poles.\nSleeps six on two queen air beds.')
      expect(p.category).toBe('Sports & Fitness > Camping > Tents')
      expect(p.warnings.join(' ')).not.toMatch(/ambiguous/)
      expect(m['shipping']?.value).toBe('Shipping & Handling Included')
    },
  },
  bedbathandbeyond: {
    title: 'Linen Blend Duvet Cover Set',
    price: '89.99',
    currency: 'USD',
    images: ['https://ak1.ostkcdn.com/imgs/55667788/duvet-1.jpg', 'https://ak1.ostkcdn.com/imgs/55667788/duvet-2.jpg'],
    variants: { count: 2, first: { sku: '55667788-001', title: 'Queen / Sand', options: { Size: 'Queen', Color: 'Sand' }, price_amount: '89.99', currency: 'USD', availability: 'in stock' } },
    specs: { Material: '55% linen, 45% cotton', Closure: 'Button' },
    extra: (p, m) => {
      expect(p.brand).toBe('Willow Loft')
      expect(p.category).toBe('Bedding > Duvet Covers')
      expect(p.variants[1]).toMatchObject({ options: { Size: 'King', Color: 'Sand' }, availability: 'out of stock' })
      expect(m['list_price']?.value).toBe('129.99 USD')
    },
  },
  bonanza: {
    title: 'Vintage Brass Compass with Wooden Box',
    price: '34.00',
    currency: 'USD',
    images: ['https://images.bonanzastatic.example/afu/images/1234/5678/compass_main.jpg', 'https://images.bonanzastatic.example/afu/images/1234/5679/compass_box.jpg'],
    specs: { Condition: 'Used', Material: 'Brass, wood', Diameter: '6 cm' },
    extra: (p, m) => {
      expect(p.description).toBe('A working brass compass in a fitted wooden box, from an estate sale.')
      expect(p.supplier_name).toBe('Brass and Beyond')
      expect(p.supplier_url).toBe('https://www.bonanza.com/booths/brassandbeyond')
      expect(m['shipping']?.value).toBe('$6.50 shipping to United States')
    },
  },
  ltdcommodities: {
    title: 'Set of 4 Solar Lantern Stakes',
    price: '24.99',
    currency: 'USD',
    images: ['https://www.ltdcommodities.example/images/products/366778_1.jpg', 'https://www.ltdcommodities.example/images/products/366778_2.jpg'],
    specs: { Material: 'Plastic, glass', Size: '6" W x 30" H', Batteries: '1 AA rechargeable (included)' },
    extra: (p, m) => {
      expect(p.sku).toBe('366778')
      expect(p.description).toBe('Warm white lanterns charge by day and glow along paths by night.')
      expect(m['list_price']?.value).toBe('Reg. $34.99')
    },
  },
  allstarhealth: {
    title: 'Brightwell Vitamin D3 5000 IU, 120 Softgels',
    price: '9.49',
    currency: 'USD',
    images: ['https://www.allstarhealth.example/images/products/brightwell-d3-120.jpg'],
    specs: { 'Serving Size': '1 Softgel', 'Vitamin D3': '125 mcg (5000 IU)' },
    extra: (p, m) => {
      expect(p.brand).toBe('Brightwell')
      expect(p.gtin).toBe('012345678905')
      expect(p.availability).toBe('in stock')
      expect(m['list_price']?.value).toBe('Retail: $15.99')
    },
  },
  flipkart: {
    title: 'Novaphone X2 (Midnight Black, 128 GB) (8 GB RAM)',
    price: '17999',
    currency: 'INR',
    images: [
      'http://rukminim2.flixcart.example/image/832/832/xif0q/mobile/a/b/c/x2-black-1.jpeg?q=90',
      'http://rukminim2.flixcart.example/image/832/832/xif0q/mobile/a/b/c/x2-black-2.jpeg?q=90',
    ],
    variants: { count: 2, first: { sku: 'MOBG123ABC', options: { Color: 'Midnight Black', Storage: '128 GB' }, availability: 'in stock' } },
    specs: { 'Model Number': 'NX2-128', 'Display Size': '16.94 cm (6.67 inch)', Resolution: '2400 x 1080, Full HD+' },
    extra: (p, m) => {
      expect(p.brand).toBe('Novaphone')
      expect(p.model).toBe('NX2-128')
      expect(p.category).toBe('Home > Mobiles & Accessories > Mobiles')
      expect(p.supplier_name).toBe('RetailNet Mobiles')
      expect(m['list_price']?.value).toBe('21999 INR')
      expect(m['rating_count']?.value).toBe('15234')
    },
  },
  snapdeal: {
    title: "Strider Men's Running Shoes, Grey",
    price: '649',
    currency: 'INR',
    images: ['https://n3.sdlcdn.example/imgs/k/a/b/strider-1.jpg', 'https://n3.sdlcdn.example/imgs/k/a/b/strider-2.jpg'],
    specs: { 'Upper Material': 'Mesh', 'Sole Material': 'EVA', Closure: 'Lace-up' },
    extra: (p, m) => {
      expect(p.brand).toBe('Strider')
      expect(p.sku).toBe('SDL123456789')
      expect(p.supplier_name).toBe('Strider Shoes')
      expect(p.supplier_url).toBe('https://www.snapdeal.com/seller/stridershoes')
      expect(m['list_price']?.value).toBe('MRP Rs. 1,499')
    },
  },
  lazada: {
    title: 'Bamboo Charcoal Memory Foam Pillow',
    price: '29.90',
    currency: 'SGD',
    images: ['https://sg-test-11.slatic.example/p/pillow-1.jpg', 'https://sg-test-11.slatic.example/p/pillow-2.jpg'],
    variants: { count: 2, first: { sku: '3011', options: { Size: 'Standard' }, price_amount: '29.90', currency: 'SGD', availability: 'in stock', image_url: 'https://sg-test-11.slatic.example/p/pillow-standard.jpg' } },
    specs: { Material: 'Memory foam', Cover: 'Bamboo charcoal fabric', Warranty: '1 year' },
    extra: (p, m) => {
      expect(p.brand).toBe('SleepWell Co')
      expect(p.description).toBe('Contoured memory foam with a breathable bamboo charcoal cover.')
      expect(p.category).toBe('Home & Living > Bedding > Pillows')
      expect(p.supplier_name).toBe('SleepWell Official')
      expect(p.supplier_url).toBe('https://www.lazada.sg/shop/sleepwell-official')
      expect(p.variants[1]).toMatchObject({ options: { Size: 'King' }, price_amount: '34.90', availability: 'out of stock' })
      expect(m['list_price']?.value).toBe('S$ 59.90')
    },
  },
  shopee: {
    title: 'Portable Mini Blender 400 ml USB Rechargeable',
    price: '15.90',
    currency: 'SGD',
    images: ['https://down-sg.img.susercontent.example/file/sg-11134207-blender1', 'https://down-sg.img.susercontent.example/file/sg-11134207-blender2'],
    specs: { Brand: 'BlendGo', Capacity: '400 ml', 'Ships From': 'Singapore' },
    extra: (p, m) => {
      expect(p.brand).toBe('BlendGo')
      expect(p.sku).toBe('7890123')
      expect(p.supplier_name).toBe('BlendGo Official Store')
      expect(p.supplier_url).toBe('https://shopee.sg/blendgo.official')
      expect(m['rating_count']?.value).toBe('1532')
    },
  },
  amazon: {
    title: 'Trailcraft Ultralight Camping Hammock with Tree Straps, Single',
    price: '29.99',
    currency: 'USD',
    images: ['https://m.media-amazon.example/images/I/71hammock1.jpg', 'https://m.media-amazon.example/images/I/81hammock2.jpg'],
    variants: { count: 3, first: { sku: 'B0TESTHAMK', title: 'Forest Green / Single', options: { Color: 'Forest Green', Size: 'Single' } } },
    specs: { Brand: 'Trailcraft', Material: 'Ripstop nylon', 'Item Weight': '340 Grams' },
    extra: (p, m) => {
      expect(p.brand).toBe('Trailcraft')
      expect(p.sku).toBe('B0TESTHAMK')
      expect(p.availability).toBe('In Stock')
      expect(p.description).toBe('Weighs 340 g and packs to the size of a grapefruit.\nHolds up to 180 kg with two 3 m tree straps.')
      expect(p.category).toBe('Sports & Outdoors > Camping & Hiking > Hammocks')
      expect(p.supplier_name).toBe('Trailcraft Outdoors')
      expect(p.supplier_url).toBe('https://www.amazon.com/gp/help/seller/at-a-glance.html?seller=A1TESTSELLER')
      expect(p.warnings.join(' ')).not.toMatch(/ambiguous/)
      expect(p.variants[2]).toMatchObject({ sku: 'B0TESTHAMS', options: { Color: 'Sunset Orange', Size: 'Single' } })
      expect(m['list_price']?.value).toBe('$39.99')
      expect(m['rating']?.value).toBe('4.6 out of 5 stars')
      expect(m['rating_count']?.value).toBe('2,318 ratings')
      expect(m['shipping']?.value).toBe('FREE delivery Tuesday, October 20')
    },
  },
  ebay: {
    title: 'Retro Bluetooth Turntable with Built-in Speakers',
    price: '64.95',
    currency: 'USD',
    images: ['https://i.ebayimg.example/images/g/AbCdEf/s-l1600.jpg', 'https://i.ebayimg.example/images/g/GhIjKl/s-l1600.jpg'],
    specs: { Condition: 'New', Speed: '33 1/3, 45, 78 RPM', Connectivity: 'Bluetooth, RCA' },
    extra: (p, m) => {
      expect(p.brand).toBe('SpinWell')
      expect(p.mpn).toBe('SW-TT20')
      expect(p.gtin).toBe('0712345678901')
      expect(p.category).toBe('Consumer Electronics > TV, Video & Home Audio > Record Players')
      expect(p.supplier_name).toBe('spinwell_audio')
      expect(p.supplier_url).toBe('https://www.ebay.com/str/spinwellaudio')
      expect(m['shipping']?.value).toBe('US $9.99 Standard Shipping')
    },
  },
  nordstrom: {
    title: 'Merino Wool Crewneck Sweater',
    price: '89.00',
    currency: 'USD',
    images: ['https://n.nordstrommedia.example/it/abc-grey-1.jpeg', 'https://n.nordstrommedia.example/it/abc-grey-2.jpeg', 'https://n.nordstrommedia.example/it/abc-navy-1.jpeg'],
    variants: { count: 2, first: { sku: '38123401', options: { Color: 'Heather Grey', Size: 'M' }, price_amount: '89.00', currency: 'USD', availability: 'in stock' } },
    specs: { Fiber: '100% merino wool', Care: 'Machine wash cold, dry flat' },
    extra: (p, m) => {
      expect(p.brand).toBe('Fjordline')
      expect(p.model).toBe('FJ-2210')
      expect(p.category).toBe('Men > Clothing > Sweaters')
      expect(p.variants[1]).toMatchObject({ options: { Color: 'Navy', Size: 'L' }, availability: 'out of stock' })
      expect(m['list_price']?.value).toBe('$129.00')
      expect(m['rating_count']?.value).toBe('57')
    },
  },
}

const REQUIRED = [
  'aliexpress', 'alibaba-com', 'alibaba-1688', 'taobao', 'tmall', 'jd', 'dhgate', 'cjdropshipping', 'banggood', 'geekbuying', 'tomtop',
  'saleyee', 'temu', 'shein', 'rosegal', 'wish', 'walmart', 'etsy', 'kohls', 'costco', 'bedbathandbeyond', 'bonanza', 'ltdcommodities',
  'allstarhealth', 'flipkart', 'snapdeal', 'lazada', 'shopee', 'amazon', 'ebay', 'nordstrom',
]

const SECRETS = ['should-not-leak', 'should-never-be-read']

const defs = marketplaceDefinitions()

describe('marketplace definitions', () => {
  it('cover every supported marketplace', () => {
    expect(defs.map((d) => d.key).sort()).toEqual([...REQUIRED].sort())
  })

  it.each(defs.map((d) => [d.key, d] as const))('%s is valid, honest about verification and matches its own fixture URL', (_key, d) => {
    expect(parseRuleDocument(d.rules).ok).toBe(true)
    expect(d.verification.status).toBe('synthetic_fixture')
    expect(d.verification.note.length).toBeGreaterThan(40)
    expect(d.verification.fixture).toBe(`testdata/fixtures/marketplaces/${d.key}.html`)
    expect(existsSync(path.join(ROOT, d.verification.fixture))).toBe(true)
    expect(domainMatches(d.domains, new URL(d.verification.fixture_url).hostname)).toBe(true)
    expect(d.domains.length).toBeLessThanOrEqual(100)
    expect(['global', 'china', 'cross_border', 'north_america', 'south_asia', 'southeast_asia']).toContain(d.region)
  })
})

describe.each(defs.map((d) => [d.key, d] as const))('%s fixture', (key, d) => {
  const html = readFileSync(path.join(ROOT, d.verification.fixture), 'utf8')
  const cfg = rulesAdapter(d)
  const { page, outputs } = analyze(html, d.verification.fixture_url, [...BUILTIN_ADAPTERS, cfg])
  const p = page.product.payload
  const own = outputs.find((o) => o.cfg.key === key)!.output
  const want = EXPECTED[key]

  it('has expectations', () => {
    expect(want, `add expectations for ${key}`).toBeDefined()
  })
  if (!want) return

  it('extracts title, price and currency with evidence from the definition', () => {
    expect(own.matched).toBe(true)
    expect(own.productSignal).toBe(true)
    expect(p.title).toBe(want.title)
    expect(page.product.fields.title?.adapter).toBe(key)
    expect(p.evidence['title']).toMatch(new RegExp(`^${key}: .*\\(rule title\\[\\d\\]\\)$`))
    expect(p.price_amount).toBe(want.price)
    expect(p.currency).toBe(want.currency)
    expect(page.pageType).toBe('product')
    expect(page.inspector.platform?.key).toBe(key)
  })

  it('extracts images', () => {
    expect(p.images.map((i) => i.url)).toEqual(want.images)
  })

  if (want.variants) {
    it('extracts variants', () => {
      expect(p.variants).toHaveLength(want.variants!.count)
      expect(p.variants[0]).toMatchObject(want.variants!.first)
    })
  }

  if (want.specs) {
    it('extracts specifications', () => {
      expect(p.specifications).toMatchObject(want.specs!)
    })
  }

  if (want.extra) {
    it('extracts the other fields its layout has', () => {
      want.extra!(p, page.product.metadata as Record<string, { value: string }>)
    })
  }

  it('declares the fields it extracts', () => {
    const fields = ruleFieldsOf(cfg.rules!)
    expect(fields).toContain('title')
    expect(fields).toContain('price')
    expect(fields).toContain('images')
  })

  it('never carries tokens, hidden inputs or form values', () => {
    const json = JSON.stringify({ page, outputs })
    for (const s of SECRETS) expect(json).not.toContain(s)
  })

  it('does not claim non-product pages on the same site', () => {
    const listing = `<!doctype html><html><head><title>Search results - ${d.name}</title></head><body><h1>Results for "lamp"</h1>
      <ul><li><a href="/p/1">Lamp one</a> $10.00</li><li><a href="/p/2">Lamp two</a> $12.00</li></ul></body></html>`
    const r = analyze(listing, d.verification.fixture_url.replace(/\/[^/]*$/, '/search?q=lamp'), [...BUILTIN_ADAPTERS, cfg])
    const o = r.outputs.find((x) => x.cfg.key === key)!.output
    expect(o.matched).toBe(false)
    expect(Object.keys(o.fields)).toEqual([])
  })
})
