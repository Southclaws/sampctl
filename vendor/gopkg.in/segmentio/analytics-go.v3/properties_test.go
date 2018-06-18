package analytics

import (
	"reflect"
	"testing"
)

func TestPropertiesSimple(t *testing.T) {
	text := "ABC"
	number := 0.5
	products := []Product{
		Product{
			ID:    "1",
			SKU:   "1",
			Name:  "A",
			Price: 42.0,
		},
		Product{
			ID:    "2",
			SKU:   "2",
			Name:  "B",
			Price: 100.0,
		},
	}

	tests := map[string]struct {
		ref Properties
		run func(Properties)
	}{
		"revenue":  {Properties{"revenue": number}, func(p Properties) { p.SetRevenue(number) }},
		"currency": {Properties{"currency": text}, func(p Properties) { p.SetCurrency(text) }},
		"value":    {Properties{"value": number}, func(p Properties) { p.SetValue(number) }},
		"path":     {Properties{"path": text}, func(p Properties) { p.SetPath(text) }},
		"referrer": {Properties{"referrer": text}, func(p Properties) { p.SetReferrer(text) }},
		"title":    {Properties{"title": text}, func(p Properties) { p.SetTitle(text) }},
		"url":      {Properties{"url": text}, func(p Properties) { p.SetURL(text) }},
		"name":     {Properties{"name": text}, func(p Properties) { p.SetName(text) }},
		"category": {Properties{"category": text}, func(p Properties) { p.SetCategory(text) }},
		"sku":      {Properties{"sku": text}, func(p Properties) { p.SetSKU(text) }},
		"price":    {Properties{"price": number}, func(p Properties) { p.SetPrice(number) }},
		"id":       {Properties{"id": text}, func(p Properties) { p.SetProductId(text) }},
		"orderId":  {Properties{"orderId": text}, func(p Properties) { p.SetOrderId(text) }},
		"total":    {Properties{"total": number}, func(p Properties) { p.SetTotal(number) }},
		"subtotal": {Properties{"subtotal": number}, func(p Properties) { p.SetSubtotal(number) }},
		"shipping": {Properties{"shipping": number}, func(p Properties) { p.SetShipping(number) }},
		"tax":      {Properties{"tax": number}, func(p Properties) { p.SetTax(number) }},
		"discount": {Properties{"discount": number}, func(p Properties) { p.SetDiscount(number) }},
		"coupon":   {Properties{"coupon": text}, func(p Properties) { p.SetCoupon(text) }},
		"products": {Properties{"products": products}, func(p Properties) { p.SetProducts(products...) }},
		"repeat":   {Properties{"repeat": true}, func(p Properties) { p.SetRepeat(true) }},
	}

	for name, test := range tests {
		prop := NewProperties()
		test.run(prop)

		if !reflect.DeepEqual(prop, test.ref) {
			t.Errorf("%s: invalid properties produced: %#v\n", name, prop)
		}
	}
}

func TestPropertiesMulti(t *testing.T) {
	p0 := Properties{"title": "A", "value": 0.5}
	p1 := NewProperties().SetTitle("A").SetValue(0.5)

	if !reflect.DeepEqual(p0, p1) {
		t.Errorf("invalid properties produced by chained setters:\n- expected %#v\n- found: %#v", p0, p1)
	}

}
