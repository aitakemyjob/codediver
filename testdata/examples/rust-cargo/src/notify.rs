//! Tells the warehouse about new orders. It reads the clock, the
//! environment and the network directly, which is what makes it hard to
//! test.

use std::env;
use std::io::Write;
use std::net::TcpStream;
use std::time::{SystemTime, UNIX_EPOCH};

use crate::order::Order;

/// Posts an order to the warehouse webhook (WAREHOUSE_ADDR, host:port).
pub fn send(o: &Order) -> std::io::Result<()> {
    let Ok(addr) = env::var("WAREHOUSE_ADDR") else {
        return Ok(());
    };
    let sent_at = SystemTime::now().duration_since(UNIX_EPOCH).unwrap_or_default().as_secs();
    let body = format!("{{\"id\":\"{}\",\"sentAt\":{}}}", o.id, sent_at);
    let mut conn = TcpStream::connect(addr)?;
    write!(
        conn,
        "POST /orders HTTP/1.1\r\nContent-Type: application/json\r\nContent-Length: {}\r\n\r\n{}",
        body.len(),
        body
    )
}
