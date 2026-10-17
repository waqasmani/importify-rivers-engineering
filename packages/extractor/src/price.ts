// Price parsing that mirrors the server's normalize.ParsePrice so the extension shows the same
// amount and currency that REA will store.

const CURRENCY_SYMBOLS: { sym: string; code: string; ambiguous: boolean }[] = [
  { sym: 'US$', code: 'USD', ambiguous: false }, { sym: 'USD', code: 'USD', ambiguous: false },
  { sym: 'C$', code: 'CAD', ambiguous: false }, { sym: 'CA$', code: 'CAD', ambiguous: false },
  { sym: 'A$', code: 'AUD', ambiguous: false }, { sym: 'AU$', code: 'AUD', ambiguous: false },
  { sym: 'NZ$', code: 'NZD', ambiguous: false }, { sym: 'HK$', code: 'HKD', ambiguous: false },
  { sym: 'S$', code: 'SGD', ambiguous: false }, { sym: 'R$', code: 'BRL', ambiguous: false },
  { sym: 'MX$', code: 'MXN', ambiguous: false }, { sym: '€', code: 'EUR', ambiguous: false },
  { sym: '£', code: 'GBP', ambiguous: false }, { sym: '₹', code: 'INR', ambiguous: false },
  { sym: '₩', code: 'KRW', ambiguous: false }, { sym: '₽', code: 'RUB', ambiguous: false },
  { sym: '₺', code: 'TRY', ambiguous: false }, { sym: '₫', code: 'VND', ambiguous: false },
  { sym: '฿', code: 'THB', ambiguous: false }, { sym: '₱', code: 'PHP', ambiguous: false },
  { sym: '₦', code: 'NGN', ambiguous: false }, { sym: 'zł', code: 'PLN', ambiguous: false },
  { sym: 'kr', code: 'SEK', ambiguous: true }, { sym: 'CHF', code: 'CHF', ambiguous: false },
  { sym: 'RMB', code: 'CNY', ambiguous: false }, { sym: 'CN¥', code: 'CNY', ambiguous: false },
  { sym: 'JP¥', code: 'JPY', ambiguous: false }, { sym: '¥', code: 'CNY', ambiguous: true },
  { sym: '元', code: 'CNY', ambiguous: false }, { sym: 'Rs', code: 'INR', ambiguous: true },
  { sym: 'AED', code: 'AED', ambiguous: false }, { sym: 'SAR', code: 'SAR', ambiguous: false },
  { sym: 'PKR', code: 'PKR', ambiguous: false }, { sym: '$', code: 'USD', ambiguous: true },
]

export const KNOWN_CURRENCIES = new Set(
  `AED AFN ALL AMD ANG AOA ARS AUD AWG AZN BAM BBD BDT BGN BHD BIF BMD BND BOB BRL BSD
BTN BWP BYN BZD CAD CDF CHF CLP CNY COP CRC CUP CVE CZK DJF DKK DOP DZD EGP ERN ETB EUR FJD FKP GBP GEL GHS GIP GMD
GNF GTQ GYD HKD HNL HTG HUF IDR ILS INR IQD IRR ISK JMD JOD JPY KES KGS KHR KMF KPW KRW KWD KYD KZT LAK LBP LKR LRD
LSL LYD MAD MDL MGA MKD MMK MNT MOP MRU MUR MVR MWK MXN MYR MZN NAD NGN NIO NOK NPR NZD OMR PAB PEN PGK PHP PKR PLN
PYG QAR RON RSD RUB RWF SAR SBD SCR SDG SEK SGD SHP SLE SOS SRD SSP STN SVC SYP SZL THB TJS TMT TND TOP TRY TTD TWD
TZS UAH UGX USD UYU UZS VES VND VUV WST XAF XCD XOF XPF YER ZAR ZMW ZWL`.split(/\s+/),
)

export interface ParsedPrice {
  amount: string
  currency: string
  warnings: string[]
}

/** Matches a visible price: an optional currency marker followed or preceded by a number. */
export const PRICE_PATTERN =
  /(?:US\$|USD|C\$|CA\$|A\$|AU\$|NZ\$|HK\$|S\$|R\$|MX\$|€|£|₹|₩|₽|₺|₫|฿|₱|₦|CHF|RMB|CN¥|JP¥|¥|\$|EUR|GBP|JPY|CNY|AUD|CAD)\s?\d[\d\s.,'’]*\d|\d[\d.,'’]*\d?\s?(?:€|EUR|USD|GBP|zł|kr|CHF|PLN|SEK|NOK|DKK|CZK|元)/

export function validCurrency(code: string): boolean {
  return KNOWN_CURRENCIES.has(code)
}

export function parsePrice(input: string, fallbackCurrency = ''): ParsedPrice | null {
  const s = input.replace(/ /g, ' ').trim()
  if (!s) return null
  const res: ParsedPrice = { amount: '', currency: '', warnings: [] }
  const upper = s.toUpperCase()
  for (const cs of CURRENCY_SYMBOLS) {
    if (upper.includes(cs.sym.toUpperCase())) {
      res.currency = cs.code
      if (cs.ambiguous) res.warnings.push(`currency symbol "${cs.sym}" is ambiguous; assumed ${cs.code}`)
      break
    }
  }
  if (!res.currency) {
    for (const f of upper.split(/[^A-Z]+/)) {
      if (f.length === 3 && KNOWN_CURRENCIES.has(f)) {
        res.currency = f
        break
      }
    }
  }
  if (!res.currency && fallbackCurrency) res.currency = fallbackCurrency
  const nums = s.match(/\d[\d\s.,'’]*/g)
  if (!nums || nums.length === 0) return null
  if (nums.length > 1 && (s.includes('-') || s.includes('–') || / to /i.test(s))) {
    res.warnings.push('price is a range; the lowest value was used')
  }
  const amount = parseNumber(nums[0] ?? '')
  if (amount === null) return null
  res.amount = amount
  return res
}

/** Interprets thousands and decimal separators in either US or European style. */
export function parseNumber(raw: string): string | null {
  let n = raw.trim().replace(/[\s'’]/g, '')
  n = n.replace(/[.,]+$/, '')
  const lastDot = n.lastIndexOf('.')
  const lastComma = n.lastIndexOf(',')
  const count = (ch: string) => n.split(ch).length - 1
  if (lastDot >= 0 && lastComma >= 0) {
    if (lastComma > lastDot) n = n.replace(/\./g, '').replace(',', '.')
    else n = n.replace(/,/g, '')
  } else if (lastComma >= 0) {
    if (n.length - lastComma - 1 === 3 && count(',') >= 1 && !(count(',') === 1 && n.length <= 4)) n = n.replace(/,/g, '')
    else if (count(',') === 1) n = n.replace(',', '.')
    else n = n.replace(/,/g, '')
  } else if (lastDot >= 0 && count('.') > 1) {
    n = n.replace(/\./g, '')
  }
  if (!/^\d+(\.\d+)?$/.test(n)) return null
  return trimAmount(n)
}

/** Normalizes a decimal string: strips leading zeros and keeps at most four decimals. */
export function trimAmount(n: string): string {
  const [intPart = '0', frac = ''] = n.split('.')
  const i = intPart.replace(/^0+(?=\d)/, '')
  const f = frac.slice(0, 4)
  return f ? `${i}.${f}` : i
}

/** Converts a JSON number or numeric string (as used in structured data) to a decimal string. */
export function amountFromStructured(value: unknown): string | null {
  if (typeof value === 'number' && Number.isFinite(value) && value >= 0) return trimAmount(String(value))
  if (typeof value === 'string') {
    const v = value.trim()
    if (/^\d+(\.\d+)?$/.test(v)) return trimAmount(v)
    const p = parsePrice(v)
    return p ? p.amount : null
  }
  return null
}
