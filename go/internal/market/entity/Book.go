package entity

import (
	"container/heap"
	"sync"
)

type Book struct {
	Order            []*Order
	Transactions     []*Transaction
	OrdersChannel    chan *Order
	OrdersChannelOut chan *Order
	Wg               *sync.WaitGroup
}

func NewBook(orderChan chan *Order, orderChanOut chan *Order, wg *sync.WaitGroup) *Book {
	return &Book{
		Order:            []*Order{},
		Transactions:     []*Transaction{},
		OrdersChannel:    orderChan,
		OrdersChannelOut: orderChanOut,
		Wg:               wg,
	}
}

func (b *Book) Trade() {
	/** Inicia as duas filas, a de compra e a de venda **/
	buyOrdersQueue := NewOrderQueue()
	sellOrdersQueue := NewOrderQueue()

	/** Joga essas filas criadas na memória heap para otimizar o algoritmo **/
	heap.Init(buyOrdersQueue)
	heap.Init(sellOrdersQueue)

	/** Loop infinito recebendo uma order do nosso canal de entrada **/
	for order := range b.OrdersChannel {
		if order.OrderType == "BUY" {
			buyOrdersQueue.Push(order)

			if sellOrdersQueue.Len() > 0 && sellOrdersQueue.Orders[0].Price <= order.Price {
				sellOrderToNegotiate := sellOrdersQueue.Pop().(*Order)

				if sellOrderToNegotiate.PendingShares > 0 {
					transaction := NewTransaction(sellOrderToNegotiate, order, order.Shares, sellOrderToNegotiate.Price)
					b.AddTransaction(transaction, b.Wg)
					sellOrderToNegotiate.Transactions = append(sellOrderToNegotiate.Transactions, transaction)
					order.Transactions = append(order.Transactions, transaction)
					b.OrdersChannelOut <- sellOrderToNegotiate
					b.OrdersChannelOut <- order
					if sellOrderToNegotiate.PendingShares > 0 {
						sellOrdersQueue.Push(sellOrderToNegotiate)
					}
				}
			}
		} else if order.OrderType == "SELL" {
			sellOrdersQueue.Push(order)

			if buyOrdersQueue.Len() > 0 && buyOrdersQueue.Orders[0].Price >= order.Price {
				buyOrderToNegotiate := buyOrdersQueue.Pop().(*Order)

				if buyOrderToNegotiate.PendingShares > 0 {
					transaction := NewTransaction(buyOrderToNegotiate, order, order.Shares, buyOrderToNegotiate.Price)
					b.AddTransaction(transaction, b.Wg)
					buyOrderToNegotiate.Transactions = append(buyOrderToNegotiate.Transactions, transaction)
					order.Transactions = append(order.Transactions, transaction)
					b.OrdersChannelOut <- buyOrderToNegotiate
					b.OrdersChannelOut <- order

					if buyOrderToNegotiate.PendingShares > 0 {
						buyOrdersQueue.Push(buyOrderToNegotiate)
					}
				}
			}
		}
	}
}

func (b *Book) AddTransaction(transaction *Transaction, wg *sync.WaitGroup) {
	defer wg.Done() // garante que será rodado somente no final de tudo nessa função

	sellingShares := transaction.SellingOrder.PendingShares
	buyingShares := transaction.BuyingOrder.PendingShares

	minShares := sellingShares

	if buyingShares < sellingShares {
		minShares = buyingShares
	}

	/** Atualiza as ações do investidor que está vendendo, diminuindo sua quantidade **/
	transaction.SellingOrder.Investor.UpdateAssetPosition(transaction.SellingOrder.Asset.ID, -minShares)
	transaction.UpdateSellOrderPendingShares(-minShares)

	/** Atualiza as ações do investidor que está comprando, aumentando sua quantidade **/
	transaction.BuyingOrder.Investor.UpdateAssetPosition(transaction.BuyingOrder.Asset.ID, minShares)
	transaction.UpdateBuyOrderPendingShares(-minShares) // Nesse caso, diminui, porque ele comprou as shares e requer menor quantidade do total

	transaction.CalculateTotal(transaction.Shares, transaction.BuyingOrder.Price)

	transaction.CloseBuyOrderTransaction()
	transaction.CloseSellOrderTransaction()

	b.Transactions = append(b.Transactions, transaction)
}
