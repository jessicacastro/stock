package entity

import (
	"container/heap"
	"sync"
)

type Book struct {
	Order         []*Order
	Transactions  []*Transaction
	OrdersChan    chan *Order // input
	OrdersChanOut chan *Order
	Wg            *sync.WaitGroup
}

func NewBook(orderChan chan *Order, orderChanOut chan *Order, wg *sync.WaitGroup) *Book {
	return &Book{
		Order:         []*Order{},
		Transactions:  []*Transaction{},
		OrdersChan:    orderChan,
		OrdersChanOut: orderChanOut,
		Wg:            wg,
	}
}

func (b *Book) Trade() {
	buyOrdersQueue := make(map[string]*OrderQueue)
	sellOrdersQueue := make(map[string]*OrderQueue)

	for order := range b.OrdersChan {
		asset := order.Asset.ID

		if buyOrdersQueue[asset] == nil {
			buyOrdersQueue[asset] = NewOrderQueue()
			heap.Init(buyOrdersQueue[asset])
		}

		if sellOrdersQueue[asset] == nil {
			sellOrdersQueue[asset] = NewOrderQueue()
			heap.Init(sellOrdersQueue[asset])
		}

		if order.OrderType == "BUY" {
			buyOrdersQueue[asset].Push(order)
			if sellOrdersQueue[asset].Len() > 0 && sellOrdersQueue[asset].Orders[0].Price <= order.Price {
				sellOrder := sellOrdersQueue[asset].Pop().(*Order)
				if sellOrder.PendingShares > 0 {
					transaction := NewTransaction(sellOrder, order, order.Shares, sellOrder.Price)
					b.AddTransaction(transaction, b.Wg)
					sellOrder.Transactions = append(sellOrder.Transactions, transaction)
					order.Transactions = append(order.Transactions, transaction)
					b.OrdersChanOut <- sellOrder
					b.OrdersChanOut <- order
					if sellOrder.PendingShares > 0 {
						sellOrdersQueue[asset].Push(sellOrder)
					}
				}
			}
		} else if order.OrderType == "SELL" {
			sellOrdersQueue[asset].Push(order)
			if buyOrdersQueue[asset].Len() > 0 && buyOrdersQueue[asset].Orders[0].Price >= order.Price {
				buyOrder := buyOrdersQueue[asset].Pop().(*Order)
				if buyOrder.PendingShares > 0 {
					transaction := NewTransaction(order, buyOrder, order.Shares, buyOrder.Price)
					b.AddTransaction(transaction, b.Wg)
					buyOrder.Transactions = append(buyOrder.Transactions, transaction)
					order.Transactions = append(order.Transactions, transaction)
					b.OrdersChanOut <- buyOrder
					b.OrdersChanOut <- order
					if buyOrder.PendingShares > 0 {
						buyOrdersQueue[asset].Push(buyOrder)
					}
				}
			}
		}
	}
}

func (b *Book) AddTransaction(transaction *Transaction, wg *sync.WaitGroup) {
	defer wg.Done()

	sellingShares := transaction.SellingOrder.PendingShares
	buyingShares := transaction.BuyingOrder.PendingShares

	minShares := sellingShares
	if buyingShares < minShares {
		minShares = buyingShares
	}

	transaction.SellingOrder.Investor.UpdateAssetPosition(transaction.SellingOrder.Asset.ID, -minShares)
	transaction.UpdateSellOrderPendingShares(-minShares)

	transaction.BuyingOrder.Investor.UpdateAssetPosition(transaction.BuyingOrder.Asset.ID, minShares)
	transaction.UpdateBuyOrderPendingShares(-minShares)

	transaction.CalculateTotal(transaction.Shares, transaction.BuyingOrder.Price)
	transaction.CloseBuyOrderTransaction()
	transaction.CloseSellOrderTransaction()
	b.Transactions = append(b.Transactions, transaction)
}
