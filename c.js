import fs from "fs";
import readline from "readline";
import https from "https";

const ipFile = "ip.txt";
const outFile = "ip_tq.csv";
const requestPath = "/cdn-cgi/trace";
const maxThreads = 50;
const timeout = 2000; // ms

// 读取 ip.txt
async function readIPs(file) {
  const rl = readline.createInterface({
    input: fs.createReadStream(file),
    crlfDelay: Infinity,
  });

  const ips = [];
  for await (const line of rl) {
    const parts = line.trim().split(/\s+/);
    if (parts.length === 2) {
      ips.push({ ip: parts[0], port: parts[1] });
    }
  }
  return ips;
}

// 使用 https.request 访问 trace
function fetchTrace({ ip, port }) {
  return new Promise((resolve) => {
    const start = Date.now();
    const req = https.request(
      {
        host: ip,
        port,
        path: requestPath,
        method: "GET",
        servername: "speed.cloudflare.com", // SNI
        rejectUnauthorized: true,          // 跳过证书校验
        headers: {
          Host: "speed.cloudflare.com",
          "User-Agent": "Mozilla/5.0",
        },
        timeout,
      },
      (res) => {
        let data = "";
        res.on("data", (chunk) => (data += chunk));
        res.on("end", () => {
          const match = data.match(/colo=([A-Z]+)[\s\S]*?loc=([A-Z]+)/);
          if (match) {
            resolve({
              ip,
              port,
              colo: match[1],
              loc: match[2],
              latency: Date.now() - start,
            });
          } else {
            resolve(null);
          }
        });
      }
    );

    req.on("error", () => resolve(null));
    req.on("timeout", () => {
      req.destroy();
      resolve(null);
    });
    req.end();
  });
}

// 并发执行
async function run() {
  const ips = await readIPs(ipFile);
  console.log(`读取到 ${ips.length} 个 IP，开始扫描...`);

  const results = [];
  let index = 0;

  async function worker() {
    while (true) {
      let current;
      if (index < ips.length) {
        current = ips[index++];
      } else break;

      const res = await fetchTrace(current);
      if (res) {
        console.log(`✅ 有效: ${res.ip}:${res.port} colo=${res.colo} loc=${res.loc} 延迟=${res.latency}ms`);
        results.push(res);
      } else {
        console.log(`❌ 无效: ${current.ip}:${current.port}`);
      }
    }
  }

  const workers = Array.from({ length: maxThreads }, worker);
  await Promise.all(workers);

  // 写 CSV
  const header = ["IP地址", "端口", "数据中心", "源IP位置", "延迟(ms)"];
  const lines = [header.join(",")];
  for (const r of results) {
    lines.push([r.ip, r.port, r.colo, r.loc, r.latency].join(","));
  }
  fs.writeFileSync(outFile, lines.join("\n"));

  console.log(`完成！共发现 ${results.length} 个有效 IP，结果已保存到 ${outFile}`);
}

run();
