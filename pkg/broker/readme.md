# Sample
```go
type Order struct {
    ID    int    `json:"id"`
    User  string `json:"user"`
    Total int    `json:"total"`
}

func main() {
    broker, err := NewNatsBroker()
    if err != nil {
        log.Fatal(err)
    }
    defer broker.Close()

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // ✅ ПОДПИСКА
    ch, cancelSub, err := Subscribe[Order](broker.nc, ctx, "orders.created", 100)
    if err != nil {
        log.Fatal(err)
    }
    defer cancelSub()

    // ✅ Слушаем сообщения в горутине
    go func() {
        for order := range ch {
            fmt.Printf("🔔 Получен заказ: ID=%d, User=%s, Total=%d\n", 
                order.ID, order.User, order.Total)
        }
    }()

    // ✅ ПУБЛИКАЦИЯ нескольких сообщений
    for i := 1; i <= 5; i++ {
        order := Order{
            ID:    i,
            User:  fmt.Sprintf("user%d", i),
            Total: i * 100,
        }
        
        if err := Publish(broker.nc, ctx, "orders.created", order); err != nil {
            log.Printf("publish error: %v", err)
        } else {
            fmt.Printf("📤 Отправлен заказ #%d\n", i)
        }
        
        time.Sleep(500 * time.Millisecond)
    }

    // Ждём обработки сообщений
    time.Sleep(2 * time.Second)
    fmt.Println("✅ Готово!")
}
```