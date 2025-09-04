import fs from "node:fs";
import path from "node:path";
import url from "node:url";

// 配置参数
const CONFIG = {
  // 每个国家提取数量
  perCountryCount: 0,
  // 是否过滤下载速度
  filterBySpeed: false,
  // 过滤下载速度下限，单位kb/s
  minSpeed: 0,
  // 指定要处理的CSV文件名
  targetFile: "ip_tq.csv"
};

// 获取当前脚本路径
const __filename = url.fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// CSV 列名
const COLUMNS = {
  ip: "IP地址",
  port: "端口",
  speed: "下载速度",
  datacenter: "源IP位置"
};

class CSVProcessor {
  constructor() {
    this.locations = null;
  }

  async processCSVFile() {
    try {
      const csvFilePath = path.resolve(__dirname, CONFIG.targetFile);
      const txtFilePath = path.resolve(__dirname, CONFIG.targetFile.replace(".csv", ".txt"));

      // 检查文件是否存在
      if (!fs.existsSync(csvFilePath)) {
        console.log(`未找到指定的 CSV 文件: ${CONFIG.targetFile}`);
        return;
      }

      console.log(`开始处理文件: ${CONFIG.targetFile}`);
      
      // 预先加载位置数据
      await this.loadLocations();
      
      await this.extractIpAndPort(csvFilePath, txtFilePath);
      
    } catch (error) {
      console.error("处理文件时发生错误:", error.message);
    }
  }

  async loadLocations() {
    try {
      const jsonFilePath = path.resolve(__dirname, "locations.json");
      const jsonData = await fs.promises.readFile(jsonFilePath, "utf8");
      this.locations = JSON.parse(jsonData);
      console.log("位置数据加载成功。");
    } catch (error) {
      console.error("加载位置数据失败:", error.message);
      throw error;
    }
  }

  isIPv6(ip) {
    return ip.includes(":");
  }

  formatIPv6(ip) {
    if (this.isIPv6(ip) && !ip.startsWith("[")) {
      return `[${ip}]`;
    }
    return ip;
  }

  getCountryFromDatacenter(datacenterCode) {
    if (!this.locations) {
      return "Unknown";
    }

    const location = this.locations.find(loc => loc.cca2 === datacenterCode);
    return location ? `${location.emoji}${location.country}` : "Unknown";
  }

  async extractIpAndPort(csvFilePath, txtFilePath) {
    try {
      console.log(`开始读取 CSV 文件...${csvFilePath}`);
      const data = await fs.promises.readFile(csvFilePath, "utf8");
      console.log("CSV 文件读取成功。");

      // 按行分割 CSV 内容
      const lines = data
        .split("\n")
        .map(line => line.trim())
        .filter(line => line); // 去掉空行
      
      if (lines.length < 2) {
        throw new Error("CSV 文件内容不足或格式不正确");
      }

      console.log("CSV 文件内容处理完成。");

      // 获取表头
      const headers = lines[0].split(",");
      const ipIndex = headers.indexOf(COLUMNS.ip);
      const portIndex = headers.indexOf(COLUMNS.port);
      const speedIndex = headers.indexOf(COLUMNS.speed);
      const datacenterIndex = headers.indexOf(COLUMNS.datacenter);

      if (ipIndex === -1 || portIndex === -1 || datacenterIndex === -1) {
        throw new Error(`CSV 文件缺少 ${COLUMNS.ip}、${COLUMNS.port} 或 ${COLUMNS.datacenter} 列`);
      }

      console.log("CSV 文件列索引检查通过。");

      // 处理数据行
      const ipEntries = [];
      for (let i = 1; i < lines.length; i++) {
        const fields = lines[i].split(",");
        
        // 跳过列数不足的行
        if (fields.length <= Math.max(ipIndex, portIndex, speedIndex, datacenterIndex)) {
          continue;
        }

        // 速度过滤
        if (CONFIG.filterBySpeed) {
          const speedField = fields[speedIndex];
          if (speedField) {
            const speedValue = parseFloat(speedField.replace(" kB/s", ""));
            if (speedValue <= CONFIG.minSpeed) {
              continue;
            }
          }
        }

        // 格式化IP和获取国家信息
        let ip = fields[ipIndex];
        const port = fields[portIndex];
        const datacenter = fields[datacenterIndex];
        
        ip = this.formatIPv6(ip);
        const country = this.getCountryFromDatacenter(datacenter);

        ipEntries.push({
          entry: `${ip}:${port}#${country}`,
          country
        });
      }

      console.log(`IP 和端口提取完成。共 ${ipEntries.length} 条记录`);

      // 按照国家分组并限制数量
      const grouped = {};
      const countryCounts = {};
      
      for (const { entry, country } of ipEntries) {
        if (!grouped[country]) {
          grouped[country] = [];
          countryCounts[country] = 0;
        }

        // 只有当国家记录数未达到限制时才添加
        if (CONFIG.perCountryCount === 0 || countryCounts[country] < CONFIG.perCountryCount) {
          grouped[country].push(entry);
          countryCounts[country]++;
        }
      }

      console.log("IP 和端口根据国家分组完成。");

      // 过滤掉数量不足的国家（如果需要满足最小数量）
      const validCountries = Object.keys(grouped).filter(country => 
        CONFIG.perCountryCount === 0 || grouped[country].length >= CONFIG.perCountryCount
      );

      if (validCountries.length === 0) {
        console.log("没有国家满足数量要求，跳过文件保存。");
        return;
      }

      // 生成结果
      const result = validCountries
        .sort()
        .map(country => {
          return grouped[country]
            .map((entry, index) => `${entry}${index + 1}`)
            .join("\n");
        })
        .join("\n");

      const countriesList = validCountries.sort().join("、");
      console.log(`提取国家: ${countriesList} (共 ${validCountries.length} 个国家)`);

      // 保存结果
      await fs.promises.writeFile(txtFilePath, result, "utf8");
      console.log(`已成功提取到 ${txtFilePath}`);

    } catch (error) {
      console.error("处理文件时发生错误:", error.message);
    }
  }
}

// 执行处理
async function main() {
  const processor = new CSVProcessor();
  await processor.processCSVFile();
}

main().catch(console.error);