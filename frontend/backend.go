package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"code.google.com/p/gorest"
	"github.com/fjukstad/kvik/kegg"
)

func main() {

	var ip = flag.String("ip", "", "ip to run on")
	var port = flag.String("port", ":8080", "port to run on")

	flag.Parse()
	address := *ip + *port

	serv := new(NOWACService)

	serv.GraphServers = make(map[string]string, 0)

	gorest.RegisterService(serv)

	http.HandleFunc("/pathwayGraph/", PathwayGraphHandler)
	http.Handle("/", gorest.Handle())

	log.Println("Starting server on", address)
	err := http.ListenAndServe(address, nil)
	if err != nil {
		log.Panic("Could not start rest-service:", err)
	}

}

type NOWACService struct {
	gorest.RestService `root:"/"
                        consumes:"application/json"
                        produces:"application/json"`

	getInfo gorest.EndPoint `method:"GET"
                            path:"/info/{Items:string}/{InfoType:string}"
                            output:"string"`

	getGeneVis gorest.EndPoint `method:"GET"
                            path:"/vis/{Gene:string}"
                            output:"string"`

	datastore gorest.EndPoint `method:"GET"
                                path:"/datastore/{...:string}"
                                output:"string"`

	datastorePost gorest.EndPoint `method:"POST"
                                    path:"/datastore/{...:string}"
                                    postdata:"string"`

	pathways gorest.EndPoint `method:"GET"
                                path:"/info/gene/{Gene:string}/pathways"
                                output:"string"`

	pathwayGeneCount gorest.EndPoint `method:"GET"
                                        path:"/info/gene/{Genes:string}/commonpathways"
                                        output:"string"`

	pathwayIDToName gorest.EndPoint `method:"GET"   
                                    path:"/info/pathway/{Id:string}/name"
                                    output:"string"`

	commonGenes gorest.EndPoint `method:"GET"
                                path:"/info/pathway/{Pathways:string}/commongenes"
                                output:"int"`

	resetCache gorest.EndPoint `method:"GET"
                                path:"/resetcache/"
                                output:"string"`

	geneIdFromName gorest.EndPoint `method:"GET"
									path:"/geneid/{Gene:string}"
									output:"string"`

	public gorest.EndPoint `method:"GET"
									path:"/public/{...:string}"
									output:"string"`

	GraphServers map[string]string
}

func PathwayGraphHandler(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Access-Control-Allow-Origin", "*")

	id := strings.Split(r.URL.Path, "/pathwayGraph/")[1]
	log.Println("id", id)

	graph := kegg.PathwayGraphFrom(id)

	log.Println(graph)

	b, err := json.Marshal(graph)
	if err != nil {
		log.Panic("Marshaling went bad: ", err)
	}

	w.Write(b)

}

type PWMap struct {
	Map map[string]int
}

func (serv NOWACService) GeneIdFromName(Name string) string {
	addAccessControlAllowOriginHeader(serv)

	geneId := kegg.GeneIdFromName(Name)

	log.Println("NAME,id ", Name, geneId)

	return geneId

}

// Serve whatever is in the public folder
func (serv NOWACService) Public(args ...string) string {
	addAccessControlAllowOriginHeader(serv)
	filename := ""
	for i, v := range args {
		filename += v
		if i < len(args)-1 {
			filename += "/"
		}
	}
	file, err := os.Open("public/" + filename)
	if err != nil {
		log.Println("ERROR READING PUBLIC FILE ", err)
	}
	contents, err := ioutil.ReadAll(file)
	if err != nil {
		log.Println("Error reading in public file ", err)
	}
	return string(contents)
}

func (serv NOWACService) ResetCache() string {
	addAccessControlAllowOriginHeader(serv)

	log.Println("!!!! CLEARING CACHE !!!!!")

	cmd := exec.Command("rm", "-rf", "cache")
	err := cmd.Run()
	if err != nil {
		log.Println(err)
	}

	return "ok"
}

// Returns the number of common genes shared between multiple pathways
func (serv NOWACService) CommonGenes(Pathways string) int {
	addAccessControlAllowOriginHeader(serv)

	pathwayList := strings.Split(Pathways, " ")

	allGenes := make(map[string]int)

	// Iterate over all genes from different pathways and set their count
	for _, p := range pathwayList {
		pw := kegg.GetPathway(p)
		genes := pw.Genes

		for _, g := range genes {

			count := allGenes[g]
			if count != 0 {
				allGenes[g] = count + 1

			} else {
				allGenes[g] = 1
			}
		}

	}

	// From map of all genes get the ones with count larger than 1
	var commonGenes []string
	for k, v := range allGenes {
		if v > 1 {
			commonGenes = append(commonGenes, k)
		}
	}

	return len(commonGenes)

}

func (serv NOWACService) PathwayIDToName(Id string) string {
	addAccessControlAllowOriginHeader(serv)
	return kegg.ReadablePathwayName(Id)
}

// Returns a list of pathways and the frequency of given genes. I.e.
// how many of the given genes are represented in different pathways
// Genes is a string that looks like "hsa:123+hsa:321+..."
func (serv NOWACService) PathwayGeneCount(Genes string) string {

	PathwayMap := make(map[string]int, 0)

	geneList := strings.Split(Genes, " ")

	// for every gene get its list of pathways
	for _, g := range geneList {

		geneId := strings.Split(g, ":")[1]
		gene := kegg.GetGene(geneId)
		pws := kegg.Pathways(gene)

		// for each of its pathways, increment the counter for number
		// of genes represented in this pathway.
		for _, p := range pws.Pathways {
			if PathwayMap[p] != 0 {
				PathwayMap[p]++
			} else {
				PathwayMap[p] = 1
			}
		}

	}

	b, err := json.Marshal(PathwayMap)
	if err != nil {
		log.Panic("marshaling went bad: ", err)
	}

	return string(b)
}

// Will return a list of pathways for a given gene
func (serv NOWACService) Pathways(Gene string) string {

	geneIdString := strings.Split(Gene, " ")[0]
	geneId := strings.Split(geneIdString, ":")[1]
	log.Println(geneId)
	gene := kegg.GetGene(geneId)
	pws := kegg.Pathways(gene)
	return kegg.PathwaysJSON(pws)

}

// Handles any requests to the Datastore. Will simply make the request to the
// datastore and return the result
func (serv NOWACService) Datastore(args ...string) string {

	addAccessControlAllowOriginHeader(serv)
	serv.RB().ConnectionClose()

	requestURL := serv.Context.Request().URL.Path

	// Where the datastore is running, this would be Stallo in later versions
	datastoreBaseURL := "http://127.0.0.1:8888/"

	URL := datastoreBaseURL + strings.Trim(requestURL, "/datastore")

	// NOTE: We are not caching results here, this could have been done, but
	// since we're doing work with a test dataset caching is not done.

	//NOTE: http.GET(URL) failed when the number of these calls were really
	//frequent. now trying gocache.
	resp, err := http.Get(URL)
	if err != nil {
		log.Print("request to datastore failed. ", err)
		serv.ResponseBuilder().SetResponseCode(404).Overide(true)
		return ":("
	}

	defer resp.Body.Close()

	// WARNING: int64 -> int conversion. may crash and burn if more than 2^32
	// - 1 bytes were read. Response from Datastore will typically be much
	// shorter than this, so its not an issue.
	respLength := int(resp.ContentLength)

	if respLength < 0 {
		respLength = 1024 * 10000
	}

	// Read the response from the body and return it as a string.
	//response := make([]byte, respLength)

	response, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		if err == io.EOF {
			log.Println("IT WAS END OF FILE NOTHING TO WORRY ABOUT")
		}
		log.Print("reading response from datastore failed. ", err)
		serv.ResponseBuilder().SetResponseCode(404).Overide(true)
		return ":("
	}

	// Set response code to what was returned from Datastore.
	// Will ensure that if a 404 is returned by datastore this is also passed
	// along
	serv.ResponseBuilder().SetResponseCode(resp.StatusCode).Overide(false)

	return string(response)
}

func (serv NOWACService) DatastorePost(PostData string, varArgs ...string) {
	addAccessControlAllowOriginHeader(serv)

	requestURL := serv.Context.Request().URL.Path

	// Where the datastore is running, this would be Stallo in later versions
	datastoreBaseURL := "http://localhost:8888/"

	URL := datastoreBaseURL + strings.Replace(requestURL, "/datastore/", "", -1)

	postContent := bytes.NewBufferString(PostData)

	// Perform the actual http post to the datastore
	// note that we set text as datatype. will fail miserably with anything else
	_, err := http.Post(URL, "text", postContent)
	if err != nil {
		log.Print("Post to datastore failed. ", err)
		serv.ResponseBuilder().SetResponseCode(500).Overide(true)
	}

}

func (serv NOWACService) GetGeneVis(Gene string) string {
	addAccessControlAllowOriginHeader(serv)

	log.Print("Returning the VIS code for gene: ", Gene)

	code := GeneExpression(Gene) // Barchar() // ParallelCoordinates(len(Gene))//GeneVisCode(Gene)
	return code
}

func GeneExpression(geneid string) string {
	ds := GetGeneExpression(geneid)

	// No dataset for this gene, return empty thing
	if ds == "[]" {
		return ""
	}
	// Header, containing all other js
	header := `
        <style>

        .chart div {
          font: 10px sans-serif;
          background-color: steelblue;
          text-align: right;
          padding: 3px;
          margin: 1px;
          color: white;
        }

        </style>
        <div class="chart"></div>
        <script src="http://d3js.org/d3.v3.min.js"></script>
        <script>`

	// dataset to be used, just random numbers now
	dataset := `var data = ` + ds

	// rest of the vis code
	vis := `


    var margin = {top: 30, right: 10, bottom: 0, left: 10},
        w = $("#c1").width() - margin.left - margin.right,
        h = 170 - margin.top - margin.bottom;
        var padding = 40

        var y = d3.scale.linear()
            .domain([-d3.max(data), d3.max(data)])
            .range([0, h]);
        
        var x = d3.scale.linear()
            .domain([-d3.max(data), d3.max(data)])
            .range([0,w]);

        var xAxis = d3.svg.axis()
            .scale(x)
            .ticks(0)
            .orient("bottom");

        var yAxis = d3.svg.axis()
            .scale(y)
            .ticks(6) 
            .tickFormat(function(d) { 
                return d * -1;
            })
            .outerTickSize(0)
            .orient("left"); 
        
        var svg = d3.select(".chart")
                    .append("svg")
					.attr("style", "padding-top:1em;")
                    .attr("width", w)
                    .attr("height", h)
        
       
        svg.selectAll("rect")
           .data(data)
           .enter()
           .append("rect")
            .attr("x", function(d, i) {
                //console.log(d,i)
                return padding*1.2 + i * 4;  //Bar width of 20 plus 1 for padding
            })
         .attr("y", function(d) {
             if(d>0) {
                 return h - y(d) 
             }
            return h/2;  //Height minus data value
        })
        .attr("fill", function(d){
            return color(d);
        })
        
       .attr("width", 3+"px")
       .attr("height", function(d) {
            return Math.abs(y(d) - y(0));
        })
        .on("click", function(d) {
            //console.log("clicked")
            //ShowBgInfo(info.Id,d)
        })

        .append("svg:title")
        .text(function(d) { 
            //console.log("hepp");
            //return GetBg(info.Id, d); 
        });
        

         svg.append("g")
            .attr("class", "x axis")
            .attr("transform", "translate("+padding+","+h/2+")")
            .call(xAxis);

        
        svg.append("g")
            .attr("class", "y axis")
            .attr("transform", "translate(" + padding + ",0)")
            .call(yAxis);
	
	var keggid = "hsa:"+info.Id
	console.log(info.Name) 
	var geneid = info.Name.split(",")[0]
	console.log(GetFoldChange(geneid)) 
	var avg =  parseFloat(GetFoldChange(geneid).Result[geneid])
	console.log(avg) 
    svg.append("line")
        .attr("x1", padding)
        .attr("y1", h - y(avg))
        .attr("x2", w)
        .attr("y2", h - y(avg))
        .style("stroke", "#fab");

    var std =  parseFloat(Std(info.Id))

    var stdup = avg + std
    var stddown = avg - std

    //console.log(stdup,stddown)

    svg.append("line")
        .attr("x1", padding)
        .attr("y1", h - y(stdup))
        .attr("x2", w)
        .attr("y2", h - y(stdup))
        .style("stroke-dasharray", ("3, 3"))
        .style("stroke", "a6bbc8");

    svg.append("line")
        .attr("x1", padding)
        .attr("y1", h - y(stddown))
        .attr("x2", w)
        .attr("y2", h - y(stddown))
        .style("stroke-dasharray", ("3, 3"))
        .style("stroke", "a6bbc8");


    ypos = function(y) {
        if(y > 0){
            return h - y
        }
        return y
    } 

    var sortOrder = false;
    var sortBars = function () {
        sortOrder = !sortOrder;
        
        sortItems = function (a, b) {
            //console.log(a,b)
            if (sortOrder) {
                return a.value - b.value;
            }
            return b.value - a.value;
        };

        svg.selectAll("rect")
            .sort(sortItems)
            .transition()
            .delay(function (d, i) {
            return i ;
        })
            .duration(1000)
            .attr("x", function (d, i) {
                //console.log(d,i,x(d))
                return x(d)+4; 

        });
    } 

    d3.select("#sort").on("click", sortBars);
    
                

        </script>
    `

	return header + dataset + vis

}

// Returns all information possible for different entities. This includes stuff
// like id,name,definition etc etc.
func (serv NOWACService) GetInfo(Items string, InfoType string) string {

	//TODO: implement different info types such as name/sequence/ etc

	addAccessControlAllowOriginHeader(serv)

	// get info about gene
	if strings.Contains(Items, "hsa:") {
		geneIdString := strings.Split(Items, " ")[0]
		geneId := strings.Split(geneIdString, ":")[1]

		gene := kegg.GetGene(geneId)

		return kegg.GeneJSON(gene)
	}

	// info about pathway
	if strings.Contains(Items, "hsa") {
		pathwayIdString := strings.Split(Items, " ")[0]
		pathway := kegg.GetPathway(pathwayIdString)
		return kegg.PathwayJSON(pathway)
	}

	// get info about compound
	if strings.Contains(Items, "cpd") {
		cid := strings.Split(Items, ":")[1]
		compound := kegg.GetCompound(cid)
		return compound.JSON()
	}

	return Items

}

func addAccessControlAllowOriginHeader(serv NOWACService) {
	// Allowing access control stuff
	rb := serv.ResponseBuilder()
	if serv.Context != nil {
		rb.AddHeader("Access-Control-Allow-Origin", "*")
	}
}

func parsePathwayInput(input string) []string {
	// Remove any unwanted characters
	a := strings.Replace(input, "%3A", ":", -1)
	a = strings.Replace(a, "&", "", -1)
	a = strings.Replace(a, "=", "", -1)

	// Split into separate hsa:... strings
	b := strings.Split(a, "pathwaySelect")

	// Clear out first empty item
	b = b[1:len(b)]

	return b

}

type ExprsResponse struct {
	Exprs []string
}

func GetGeneExpression(id string) string {

	datastore := "http://localhost:8888"

	query := "/exprs/" + id
	url := datastore + query
	response, err := http.Get(url)

	if err != nil {
		log.Panic("could not download expression ", err)
	} else if response.StatusCode != 200 {
		log.Println(response)
		log.Println("Error from datastore")
		return "[]"
	}

	defer response.Body.Close()

	result, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Panic("Could not read expression ", err)
	}

	exprs := new(ExprsResponse)
	err = json.Unmarshal(result, exprs)
	if err != nil {
		log.Panic(err)
	}

	// returning a string that looks like an array. sorry bout that .
	values := strings.Join(exprs.Exprs, ",")
	values = "[" + values
	values = values + "]"

	values = strings.Replace(values, "NA", "0", -1)
	return values

}
