package nacos

import (
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/nacos-group/nacos-istio/common"
	"github.com/nacos-group/nacos-sdk-go/model"
	"istio.io/api/mcp/v1alpha1"
	"istio.io/api/networking/v1alpha3"
)

type NacosService interface {
	// Subscribe all services changes in Nacos:
	SubscribeAllServices(SubscribeCallback func(resources *v1alpha1.Resources, err error))

	// Subscribe one service changes in Nacos:
	SubscribeService(ServiceName string, SubscribeCallback func(endpoints []model.SubscribeService, err error))
}

/**
 * Mocked Nacos service that sends whole set of services after a fixed delay.
 * This service tries to measure the performance of Pilot and the MCP protocol.
 */
type MockNacosService struct {
	// Running configurations:
	MockParams common.MockParams
	callbacks  []func(resources *v1alpha1.Resources, err error)
	// All mocked services:
	Resources *v1alpha1.Resources
}

func NewMockNacosService(MockParams common.MockParams) *MockNacosService {

	mns := &MockNacosService{
		MockParams: MockParams,
		callbacks:  []func(resources *v1alpha1.Resources, err error){},
	}

	mns.constructServices()

	go mns.notifyServiceChange()

	return mns
}

func (mockService *MockNacosService) SubscribeAllServices(SubscribeCallback func(resources *v1alpha1.Resources, err error)) {
	mockService.callbacks = append(mockService.callbacks, SubscribeCallback)
}

func (mockService *MockNacosService) SubscribeService(ServiceName string, SubscribeCallback func(endpoints []model.SubscribeService, err error)) {

}

/**
 * Construct all services that will be pushed to Istio
 */
func (mockService *MockNacosService) constructServices() {

	// collection := "istio/networking/v1alpha3/synthetic/serviceentries"
	// incremental := true

	collection := "istio/networking/v1alpha3/serviceentries"
	incremental := false

	mockService.Resources = &v1alpha1.Resources{
		Collection:  collection,
		Incremental: incremental,
	}

	port := &v1alpha3.Port{
		Number:   8080,
		Protocol: "HTTP",
		Name:     "http",
	}

	totalInstanceCount := 0

	labels := make(map[string]string)
	labels["p"] = "hessian2"
	labels["ROUTE"] = "0"
	labels["APP"] = "ump"
	labels["st"] = "na62"
	labels["v"] = "2.0"
	labels["TIMEOUT"] = "3000"
	labels["ih2"] = "y"
	labels["mg"] = "ump2_searchhost"
	labels["WRITE_MODE"] = "unit"
	labels["CONNECTTIMEOUT"] = "1000"
	labels["SERIALIZETYPE"] = "hessian"
	labels["ut"] = "UNZBMIX25G"

	changed := 0

	for count := 0; count < mockService.MockParams.MockServiceCount; count++ {
		svcName := mockService.MockParams.MockServiceNamePrefix + "." + strconv.Itoa(count)
		se := &v1alpha3.ServiceEntry{
			Hosts:      []string{svcName + ".nacos.svc.cluster.local"},
			Resolution: v1alpha3.ServiceEntry_STATIC,
			Location:   v1alpha3.ServiceEntry_MESH_INTERNAL,
			Ports:      []*v1alpha3.Port{port},
		}

		rand.Seed(time.Now().Unix())

		instanceCount := mockService.MockParams.MockAvgEndpointCount

		inc := int(mockService.MockParams.MockEndpointChangeRatio * float64(instanceCount))
		changed += inc

		// //0.01% of the services have large number of endpoints:
		// if count%10000 == 0 {
		// 	instanceCount = 20000
		// }

		for i := 0; i < instanceCount; i++ {
			ip := fmt.Sprintf("%d.%d.%d.%d",
				10, byte(i>>16), byte(i>>8), byte(i))

			endpoint := &v1alpha3.ServiceEntry_Endpoint{
				Labels: labels,
			}

			endpoint.Address = ip
			endpoint.Ports = map[string]uint32{
				"http": uint32(8080),
			}

			if i < inc {
				endpoint.Weight = rand.Uint32()
			} else {
				endpoint.Weight = 1
			}

			se.Endpoints = append(se.Endpoints, endpoint)
		}

		totalInstanceCount += len(se.Endpoints)

		seAny, err := types.MarshalAny(se)
		if err != nil {
			continue
		}

		res := v1alpha1.Resource{
			Body: seAny,
			Metadata: &v1alpha1.Metadata{
				Annotations: map[string]string{
					"virtual": "1",
					"networking.alpha.istio.io/serviceVersion": "1",
				},
				Name: "nacos" + "/" + svcName, // goes to model.Config.Name and Namespace - of course different syntax
			},
		}

		mockService.Resources.Resources = append(mockService.Resources.Resources, res)
	}

	log.Println("Generated", mockService.MockParams.MockServiceCount, "services.")
	log.Println(fmt.Sprintf("Total instance count %d , changed: %d", totalInstanceCount, changed))
}

func (mockService *MockNacosService) notifyServiceChange() {

	incrementalResources := &v1alpha1.Resources{
		Collection:  "istio/networking/v1alpha3/synthetic/serviceentries",
		Incremental: true,
	}
	pushServiceCount := len(mockService.Resources.Resources) * mockService.MockParams.MockIncrementalRatio / 100
	for _, resource := range mockService.Resources.Resources {
		if pushServiceCount <= 0 {
			break
		}
		pushServiceCount--
		resource.Metadata.Annotations["networking.alpha.istio.io/endpointsVersion"] = strconv.FormatInt(time.Now().UnixNano()/1000, 10)
		incrementalResources.Resources = append(incrementalResources.Resources, resource)
	}

	for {
		mockService.constructServices()

		for _, callback := range mockService.callbacks {
			if mockService.MockParams.MockTestIncremental {
				go callback(incrementalResources, nil)
			} else {
				go callback(mockService.Resources, nil)
			}
		}
		time.Sleep(time.Duration(mockService.MockParams.MockPushDelay) * time.Millisecond)
	}
}
